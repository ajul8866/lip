// Package runner is LIP's bounded autonomous component. It claims at most
// MaxConcurrent queued tasks, executes only catalogued tools, and refuses
// active tools without a recorded approval. It never shells out and never
// executes arbitrary code.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/config"
	"github.com/lip-project/lip/internal/egress"
	"github.com/lip-project/lip/internal/llm"
	"github.com/lip-project/lip/internal/store"
	"github.com/lip-project/lip/internal/toolcatalog"
)

const (
	taskTimeout  = 5 * time.Minute
	tickInterval = 2 * time.Second
	cleanupEvery = time.Minute
)

type Runner struct {
	cfg   config.Config
	store *store.Store
	audit *audit.Recorder
	llm   *llm.Registry
	log   *slog.Logger

	sem      chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func New(cfg config.Config, st *store.Store, rec *audit.Recorder, registry *llm.Registry, log *slog.Logger) *Runner {
	return &Runner{
		cfg:   cfg,
		store: st,
		audit: rec,
		llm:   registry,
		log:   log,
		sem:   make(chan struct{}, cfg.MaxConcurrent),
		stop:  make(chan struct{}),
	}
}

func (r *Runner) Start(ctx context.Context) {
	go r.loop(ctx)
}

func (r *Runner) Stop() {
	r.stopOnce.Do(func() { close(r.stop) })
}

func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) loop(ctx context.Context) {
	if requeued, err := r.store.Q().RequeueStale(ctx, 15); err != nil {
		r.log.Warn("requeue stale tasks failed", "error", err)
	} else if requeued > 0 {
		r.log.Warn("requeued stale tasks", "count", requeued)
		r.recordSystemAudit(ctx, "task.requeue", "", "success", map[string]any{"count": requeued})
	}
	if !r.cfg.AutoRun {
		r.log.Warn("autonomous runner disabled by LIP_AUTORUN=false; queued tasks will not execute")
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	cleanup := time.NewTicker(cleanupEvery)
	defer cleanup.Stop()

	r.cleanup(ctx)
	for {
		select {
		case <-r.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.cfg.AutoRun {
				r.claim(ctx)
			}
		case <-cleanup.C:
			r.cleanup(ctx)
		}
	}
}

func (r *Runner) claim(ctx context.Context) {
	for {
		select {
		case r.sem <- struct{}{}:
		default:
			return
		}
		task, err := r.store.Q().ClaimQueued(ctx)
		if err != nil {
			<-r.sem
			if !errors.Is(err, store.ErrNotFound) {
				r.log.Warn("claim task failed", "error", err)
			}
			return
		}
		r.wg.Add(1)
		go func(t store.Task) {
			defer func() {
				<-r.sem
				r.wg.Done()
			}()
			r.execute(t)
		}(task)
	}
}

type toolResult struct {
	Result   any
	Summary  string
	Findings []llm.DraftFinding
}

func (r *Runner) execute(task store.Task) {
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	q := r.store.Q()
	if _, err := q.GetProject(ctx, task.ProjectID); err != nil {
		r.fail(task, "project is unavailable", false)
		return
	}
	def, ok := toolcatalog.Get(task.Tool)
	if !ok {
		r.fail(task, "tool is not in the catalog", true)
		return
	}

	var err error
	var out toolResult
	switch task.Tool {
	case "inventory_summary":
		out, err = r.inventorySummary(ctx, task)
	case "synthesize_findings":
		out, err = r.synthesizeFindings(ctx, task)
	case "http_head":
		out, err = r.httpHead(ctx, task)
	default:
		err = fmt.Errorf("tool %s is catalogued but not implemented", def.Name)
	}
	if err != nil {
		denied := errors.Is(err, egress.ErrDenied) || errors.Is(err, llm.ErrProviderDenied)
		r.fail(task, publicError(err), denied)
		return
	}

	persistErr := r.store.InTx(ctx, func(tx *store.Queries) error {
		ids := make([]string, 0, len(out.Findings))
		for _, draft := range out.Findings {
			f := store.Finding{
				ID:          store.NewID(),
				ProjectID:   task.ProjectID,
				TaskID:      &task.ID,
				Title:       draft.Title,
				Description: draft.Description,
				Severity:    draft.Severity,
				Status:      "open",
				Source:      "llm",
				Evidence:    draft.Evidence,
			}
			created, err := tx.CreateFinding(ctx, f)
			if err != nil {
				return err
			}
			ids = append(ids, created.ID)
			if err := r.audit.Record(ctx, tx, audit.Entry{
				Action: "finding.create", TargetType: "finding", TargetID: created.ID,
				Result: "success", Metadata: map[string]any{"source": "llm", "taskId": task.ID},
			}); err != nil {
				return err
			}
		}
		result := map[string]any{"summary": out.Summary, "data": out.Result, "findingIds": ids}
		if _, err := tx.FinishTask(ctx, task.ID, "succeeded", result, ""); err != nil {
			return err
		}
		if task.ConversationID != nil {
			meta, _ := json.Marshal(map[string]any{"taskId": task.ID, "tool": task.Tool, "findingCount": len(ids)})
			if _, err := tx.CreateMessage(ctx, store.Message{
				ID:             store.NewID(),
				ConversationID: *task.ConversationID,
				Role:           "tool",
				Content:        out.Summary,
				Metadata:       meta,
			}); err != nil {
				return err
			}
			if err := tx.TouchConversation(ctx, *task.ConversationID); err != nil {
				return err
			}
		}
		return r.audit.Record(ctx, tx, audit.Entry{
			Action: "task.execute", TargetType: "task", TargetID: task.ID,
			Result: "success", Metadata: map[string]any{
				"tool": task.Tool, "risk": task.RiskLevel, "findings": len(ids),
			},
		})
	})
	if persistErr != nil {
		r.log.Error("persist task result failed", "task_id", task.ID, "error", persistErr)
		return
	}
	r.log.Info("task succeeded", "task_id", task.ID, "project_id", task.ProjectID, "tool", task.Tool)
}

func (r *Runner) fail(task store.Task, reason string, denied bool) {
	reason = truncate(reason, 1000)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result := "failure"
	if denied {
		result = "denied"
	}
	err := r.store.InTx(ctx, func(tx *store.Queries) error {
		if _, err := tx.FinishTask(ctx, task.ID, "failed", map[string]any{"error": reason}, reason); err != nil {
			return err
		}
		if task.ConversationID != nil {
			meta, _ := json.Marshal(map[string]any{"taskId": task.ID, "tool": task.Tool, "failed": true})
			if _, err := tx.CreateMessage(ctx, store.Message{
				ID: store.NewID(), ConversationID: *task.ConversationID,
				Role: "tool", Content: "Task failed: " + reason, Metadata: meta,
			}); err != nil {
				return err
			}
		}
		return r.audit.Record(ctx, tx, audit.Entry{
			Action: "task.execute", TargetType: "task", TargetID: task.ID,
			Result: result, Metadata: map[string]any{"tool": task.Tool, "reason": reason},
		})
	})
	if err != nil {
		r.log.Error("mark task failed failed", "task_id", task.ID, "error", err)
	}
	r.log.Warn("task failed", "task_id", task.ID, "reason", reason)
}

func (r *Runner) inventorySummary(ctx context.Context, task store.Task) (toolResult, error) {
	q := r.store.Q()
	assets, _, err := q.ListAssets(ctx, task.ProjectID, "", 500, 0)
	if err != nil {
		return toolResult{}, err
	}
	total, err := q.CountAssetsForProject(ctx, task.ProjectID)
	if err != nil {
		return toolResult{}, err
	}
	byKind := map[string][]string{}
	for _, a := range assets {
		if len(byKind[a.Kind]) < 8 {
			byKind[a.Kind] = append(byKind[a.Kind], a.Value)
		}
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	var b strings.Builder
	fmt.Fprintf(&b, "Declared assets: %d\n", total)
	for _, k := range kinds {
		fmt.Fprintf(&b, "- %s: %s\n", k, strings.Join(byKind[k], ", "))
	}
	if total == 0 {
		b.WriteString("No assets have been imported for this project.\n")
	}
	summary := b.String()
	return toolResult{
		Summary: summary,
		Result:  map[string]any{"assetCount": total, "kinds": kinds},
	}, nil
}

func (r *Runner) synthesizeFindings(ctx context.Context, task store.Task) (toolResult, error) {
	q := r.store.Q()
	project, err := q.GetProject(ctx, task.ProjectID)
	if err != nil {
		return toolResult{}, err
	}
	assets, _, err := q.ListAssets(ctx, task.ProjectID, "", 500, 0)
	if err != nil {
		return toolResult{}, err
	}
	var messages []store.Message
	if task.ConversationID != nil {
		total, err := q.CountMessages(ctx, *task.ConversationID)
		if err != nil {
			return toolResult{}, err
		}
		offset := int(total) - 20
		if offset < 0 {
			offset = 0
		}
		messages, _, err = q.ListMessages(ctx, *task.ConversationID, 20, offset)
		if err != nil {
			return toolResult{}, err
		}
	}
	providerID := providerIDFromParams(task.Parameters)
	drafts, cfg, err := r.llm.Synthesize(ctx, q, providerID, project, assets, messages)
	if err != nil {
		r.log.Warn("synthesize provider failed", "task_id", task.ID, "error", err)
		return toolResult{}, err
	}
	return toolResult{
		Summary:  fmt.Sprintf("Drafted %d finding(s) with provider %s (%s).", len(drafts), cfg.Name, cfg.Model),
		Result:   map[string]any{"provider": cfg.Name, "model": cfg.Model},
		Findings: drafts,
	}, nil
}

func (r *Runner) httpHead(ctx context.Context, task store.Task) (toolResult, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(task.Parameters, &params); err != nil || params.URL == "" {
		return toolResult{}, fmt.Errorf("http_head requires a url parameter")
	}
	u, err := r.llm.Guard().CheckURL(ctx, params.URL)
	if err != nil {
		return toolResult{}, err
	}
	declared, err := r.store.Q().IsDeclaredAsset(ctx, task.ProjectID, "url", u.String())
	if err != nil {
		return toolResult{}, err
	}
	if !declared {
		return toolResult{}, fmt.Errorf("target URL is not a declared asset in this project")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	if err != nil {
		return toolResult{}, fmt.Errorf("invalid request")
	}
	req.Header.Set("User-Agent", "lip-authorized-check")
	start := time.Now()
	client := r.llm.Guard().Client(15 * time.Second)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return toolResult{}, err
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)
	headers := map[string]string{}
	for _, key := range []string{"Content-Type", "Content-Length", "Server", "Location", "Strict-Transport-Security"} {
		if v := resp.Header.Get(key); v != "" {
			headers[key] = truncate(v, 500)
		}
	}
	summary := fmt.Sprintf("HEAD %s returned HTTP %d in %d ms.", u.String(), resp.StatusCode, elapsed.Milliseconds())
	return toolResult{
		Summary: summary,
		Result: map[string]any{
			"url": u.String(), "status": resp.StatusCode,
			"headers": headers, "durationMs": elapsed.Milliseconds(),
		},
	}, nil
}

func providerIDFromParams(raw json.RawMessage) string {
	var p struct {
		ProviderID string `json:"providerId"`
	}
	_ = json.Unmarshal(raw, &p)
	return p.ProviderID
}

func (r *Runner) cleanup(ctx context.Context) {
	approvals, err := r.store.Q().ExpireApprovals(ctx)
	if err != nil {
		r.log.Warn("expire approvals failed", "error", err)
		return
	}
	for _, a := range approvals {
		r.recordSystemAudit(ctx, "approval.expire", a.ID, "success", map[string]any{"taskId": a.TaskID})
		if _, err := r.store.Q().BlockTask(ctx, a.TaskID, "approval expired without a decision"); err != nil && !errors.Is(err, store.ErrNotFound) {
			r.log.Warn("block task after approval expiry failed", "task_id", a.TaskID, "error", err)
		}
	}
	if _, err := r.store.Q().DeleteExpiredSessions(ctx); err != nil {
		r.log.Warn("delete expired sessions failed", "error", err)
	}
}

func (r *Runner) recordSystemAudit(ctx context.Context, action, targetID, result string, metadata map[string]any) {
	if err := r.store.InTx(ctx, func(tx *store.Queries) error {
		return r.audit.Record(ctx, tx, audit.Entry{
			Action: action, TargetType: strings.Split(action, ".")[0], TargetID: targetID,
			Result: result, Metadata: metadata,
		})
	}); err != nil {
		r.log.Warn("system audit failed", "action", action, "error", err)
	}
}

func publicError(err error) string {
	switch {
	case errors.Is(err, llm.ErrNoProvider):
		return "No enabled LLM provider is configured. Add one in Settings."
	case errors.Is(err, llm.ErrProviderDenied):
		return "The configured LLM provider is blocked by safe local-only mode."
	case errors.Is(err, llm.ErrBadOutput):
		return "The LLM returned output that did not match the required JSON schema."
	case errors.Is(err, egress.ErrDenied):
		return "Outbound destination is blocked by the egress policy: " + egress.ErrorMessage(err)
	default:
		return truncate(err.Error(), 1000)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
