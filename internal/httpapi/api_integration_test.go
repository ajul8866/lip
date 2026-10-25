package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/config"
	"github.com/lip-project/lip/internal/db"
	"github.com/lip-project/lip/internal/egress"
	"github.com/lip-project/lip/internal/httpapi"
	"github.com/lip-project/lip/internal/i18n"
	"github.com/lip-project/lip/internal/llm"
	"github.com/lip-project/lip/internal/runner"
	"github.com/lip-project/lip/internal/store"
)

type apiClient struct {
	base   string
	client *http.Client
}

func newAPIClient(base string) *apiClient {
	jar, _ := cookiejar.New(nil)
	return &apiClient{base: base, client: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
}

func (c *apiClient) request(t *testing.T, method, path string, body any, headers map[string]string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp.StatusCode, data
}

func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return out
}

func waitFor(t *testing.T, timeout time.Duration, label string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(120 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", label)
}

func TestVerticalSlice(t *testing.T) {
	dsn := os.Getenv("LIP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LIP_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn, 5)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.Migrate(ctx, pool, logger); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var headHits atomic.Int32
	headServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		headHits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(headServer.Close)
	headURL := headServer.URL + "/"

	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/chat":
			var req struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			system := ""
			for _, message := range req.Messages {
				if message.Role == "system" {
					system = message.Content
					break
				}
			}
			content := `{"message":"noop"}`
			switch {
			case strings.Contains(system, "LIP_MODE=plan"):
				content = `{"tasks":[
					{"title":"Inventory pass","description":"Summarize declared assets.","tool":"inventory_summary","parameters":{}},
					{"title":"Draft findings","description":"Draft cautious findings.","tool":"synthesize_findings","parameters":{}}
				]}`
			case strings.Contains(system, "LIP_MODE=synthesize"):
				content = `{"findings":[{"title":"[TEST] Validate declared asset exposure","description":"Test-double finding for integration coverage.","severity":"medium","evidence":{"asset":"example.com"}}]}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]string{"content": content},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ollamaServer.Close)

	ollamaPort := strings.TrimPrefix(ollamaServer.URL, "http://")
	headPort := strings.TrimPrefix(headServer.URL, "http://")
	cfg := config.Config{
		Addr:            "127.0.0.1:0",
		DatabaseURL:     dsn,
		DBMaxConns:      5,
		SessionCookie:   "lip_session",
		SessionTTL:      time.Hour,
		SafeLocalOnly:   true,
		SafeModeSource:  "test",
		EgressAllowlist: []string{ollamaPort, headPort},
		ImportRoot:      t.TempDir(),
		MaxConcurrent:   2,
		AutoRun:         true,
		LogLevel:        slog.LevelError,
		LogFormat:       "json",
		DefaultLocale:   "en",
		ShutdownTimeout: 2 * time.Second,
		MaxRequestBytes: 2 << 20,
		SetupToken:      "setup-secret",
		Version:         "test",
	}
	guard, err := egress.New(egress.Options{Safe: true, Allowlist: cfg.EgressAllowlist})
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	bundle, err := i18n.New()
	if err != nil {
		t.Fatalf("i18n: %v", err)
	}
	st := store.New(pool)
	recorder := audit.NewRecorder(logger)
	registry := llm.NewRegistry(guard)
	taskRunner := runner.New(cfg, st, recorder, registry, logger)
	taskRunner.Start(ctx)
	t.Cleanup(func() {
		taskRunner.Stop()
		taskRunner.Wait()
	})
	server := httptest.NewServer(httpapi.New(cfg, st, guard, registry, recorder, bundle, logger, nil).Handler())
	t.Cleanup(server.Close)
	base := server.URL + "/api/v1"
	admin := newAPIClient(base)
	analyst := newAPIClient(base)

	status, data := admin.request(t, "GET", "/setup/status", nil, nil)
	if status != 200 {
		t.Fatalf("setup status: %d %s", status, data)
	}
	setupStatus := decode[struct {
		SetupRequired bool `json:"setupRequired"`
	}](t, data)
	if !setupStatus.SetupRequired {
		t.Fatal("expected setupRequired")
	}
	status, _ = admin.request(t, "POST", "/setup", map[string]any{
		"email": "admin@example.test", "password": "correct horse battery staple", "projectName": "Acme",
	}, nil)
	if status != 403 {
		t.Fatalf("setup without token: got %d", status)
	}
	status, data = admin.request(t, "POST", "/setup", map[string]any{
		"email": "admin@example.test", "password": "correct horse battery staple",
		"displayName": "Ada", "projectName": "Acme",
	}, map[string]string{"X-LIP-Setup-Token": "setup-secret"})
	if status != 201 {
		t.Fatalf("setup: %d %s", status, data)
	}
	setup := decode[struct {
		User    store.User    `json:"user"`
		Project store.Project `json:"project"`
	}](t, data)
	projectID := setup.Project.ID
	if projectID == "" || setup.User.Role != "admin" {
		t.Fatalf("unexpected setup response: %+v", setup)
	}

	status, data = admin.request(t, "POST", "/llm-providers", map[string]any{
		"name": "Local test model", "kind": "ollama", "baseUrl": ollamaServer.URL,
		"model": "test-model", "enabled": true, "isDefault": true,
	}, nil)
	if status != 201 {
		t.Fatalf("create provider: %d %s", status, data)
	}
	providerResponse := decode[struct {
		Provider store.LLMProvider `json:"provider"`
	}](t, data)
	if providerResponse.Provider.ID == "" {
		t.Fatal("provider was not created")
	}
	if _, _, err := registry.Resolve(ctx, st.Q(), providerResponse.Provider.ID); err != nil {
		t.Fatalf("provider should resolve after creation: %v", err)
	}
	if _, cfg, err := registry.Resolve(ctx, st.Q(), ""); err != nil {
		t.Fatalf("default provider should resolve after creation: %v", err)
	} else if !cfg.IsDefault {
		t.Fatalf("resolved provider is not default: %+v", cfg)
	}

	status, data = admin.request(t, "POST", "/llm-providers", map[string]any{
		"name": "Cloud", "kind": "openai", "baseUrl": "https://api.openai.com/v1",
		"model": "gpt-4o-mini", "apiKeyEnv": "LIP_LLM_API_KEY", "enabled": true,
	}, nil)
	if status != 403 {
		t.Fatalf("openai provider should be blocked in safe mode: %d %s", status, data)
	}

	status, data = admin.request(t, "POST", "/projects/"+projectID+"/assets/import", map[string]any{
		"source": "inline",
		"values": []string{"domain example.com", "url " + headURL, "ip 192.0.2.10"},
	}, nil)
	if status != 200 {
		t.Fatalf("asset import: %d %s", status, data)
	}
	status, data = admin.request(t, "POST", "/projects/"+projectID+"/assets/import", map[string]any{
		"source": "url", "url": "https://example.com/assets.txt",
	}, nil)
	if status != 403 || !strings.Contains(string(data), "safe_mode_denied") {
		t.Fatalf("remote import should be denied: %d %s", status, data)
	}

	status, data = admin.request(t, "POST", "/projects/"+projectID+"/conversations", map[string]any{"title": "Scope"}, nil)
	if status != 201 {
		t.Fatalf("create conversation: %d %s", status, data)
	}
	conversationID := decode[struct {
		Conversation store.Conversation `json:"conversation"`
	}](t, data).Conversation.ID
	status, data = admin.request(t, "POST", "/conversations/"+conversationID+"/messages", map[string]any{
		"content": "Assess declared assets only. Do not scan external targets.",
	}, nil)
	if status != 201 {
		t.Fatalf("create message: %d %s", status, data)
	}

	status, data = admin.request(t, "POST", "/conversations/"+conversationID+"/plan", map[string]any{}, nil)
	if status != 201 {
		t.Fatalf("plan: %d %s", status, data)
	}
	planResponse := decode[struct {
		Tasks []store.Task `json:"tasks"`
	}](t, data)
	if len(planResponse.Tasks) != 2 {
		t.Fatalf("expected 2 planned tasks, got %d", len(planResponse.Tasks))
	}
	waitFor(t, 8*time.Second, "passive tasks", func() bool {
		status, data := admin.request(t, "GET", "/projects/"+projectID+"/tasks?limit=50", nil, nil)
		if status != 200 {
			return false
		}
		tasks := decode[struct {
			Items []store.Task `json:"items"`
		}](t, data).Items
		for _, task := range tasks {
			if task.Status == "queued" || task.Status == "running" {
				return false
			}
		}
		return len(tasks) >= 2
	})
	status, data = admin.request(t, "GET", "/projects/"+projectID+"/findings?limit=20", nil, nil)
	if status != 200 {
		t.Fatalf("list findings: %d %s", status, data)
	}
	findings := decode[struct {
		Items []store.Finding `json:"items"`
	}](t, data).Items
	if len(findings) == 0 || findings[0].Source != "llm" {
		t.Fatalf("expected an LLM-drafted finding, got %+v", findings)
	}

	status, data = admin.request(t, "POST", "/users", map[string]any{
		"email": "analyst@example.test", "displayName": "Analyst", "role": "analyst",
		"locale": "en", "password": "another correct horse battery",
	}, nil)
	if status != 201 {
		t.Fatalf("create analyst: %d %s", status, data)
	}
	status, data = analyst.request(t, "POST", "/auth/login", map[string]any{
		"email": "analyst@example.test", "password": "another correct horse battery",
	}, nil)
	if status != 200 {
		t.Fatalf("analyst login: %d %s", status, data)
	}

	status, data = analyst.request(t, "POST", "/projects/"+projectID+"/tasks", map[string]any{
		"title": "Blocked by temp password", "tool": "inventory_summary",
	}, nil)
	if status != 403 || !strings.Contains(string(data), "password_change_required") {
		t.Fatalf("temporary password should block writes: %d %s", status, data)
	}
	status, data = analyst.request(t, "POST", "/auth/change-password", map[string]any{
		"currentPassword": "another correct horse battery",
		"newPassword":     "analyst chosen unique passphrase",
	}, nil)
	if status != 204 {
		t.Fatalf("analyst change password: %d %s", status, data)
	}
	status, data = analyst.request(t, "POST", "/projects/"+projectID+"/tasks", map[string]any{
		"title": "Head check", "tool": "http_head", "parameters": map[string]string{"url": headURL},
	}, nil)
	if status != 201 {
		t.Fatalf("create active task: %d %s", status, data)
	}
	activeTask := decode[struct {
		Task     store.Task     `json:"task"`
		Approval store.Approval `json:"approval"`
	}](t, data)
	if activeTask.Task.Status != "awaiting_approval" || activeTask.Approval.ID == "" {
		t.Fatalf("active task was not gated: %+v", activeTask)
	}
	status, data = analyst.request(t, "POST", "/approvals/"+activeTask.Approval.ID+"/decide", map[string]any{
		"decision": "approve", "reason": "self approval attempt",
	}, nil)
	if status != 403 {
		t.Fatalf("analyst self-approval should be denied: %d %s", status, data)
	}
	status, data = admin.request(t, "POST", "/approvals/"+activeTask.Approval.ID+"/decide", map[string]any{
		"decision": "approve", "reason": "reviewed scope",
	}, nil)
	if status != 200 {
		t.Fatalf("admin approval: %d %s", status, data)
	}
	waitFor(t, 8*time.Second, "active task execution", func() bool {
		status, data := admin.request(t, "GET", "/tasks/"+activeTask.Task.ID, nil, nil)
		if status != 200 {
			return false
		}
		task := decode[struct {
			Task store.Task `json:"task"`
		}](t, data).Task
		return task.Status == "succeeded" || task.Status == "failed"
	})
	if headHits.Load() == 0 {
		t.Fatal("expected the declared HEAD target to be called")
	}
	status, data = admin.request(t, "GET", "/tasks/"+activeTask.Task.ID, nil, nil)
	task := decode[struct {
		Task store.Task `json:"task"`
	}](t, data).Task
	if task.Status != "succeeded" {
		t.Fatalf("active task failed: %s %s", task.Status, task.LastError)
	}

	status, data = admin.request(t, "GET", "/projects/"+projectID+"/report", nil, nil)
	if status != 200 || !strings.Contains(string(data), "Acme") || !strings.Contains(string(data), "Validate declared asset exposure") {
		t.Fatalf("report: %d %s", status, data)
	}
	if !strings.Contains(string(data), "Approval decisions") {
		t.Fatalf("report missing approval section")
	}

	status, data = admin.request(t, "GET", "/audit?limit=100", nil, nil)
	if status != 200 {
		t.Fatalf("audit: %d %s", status, data)
	}
	auditPage := decode[struct {
		Items []store.AuditEntry `json:"items"`
	}](t, data)
	seen := map[string]bool{}
	for _, entry := range auditPage.Items {
		seen[entry.Action] = true
	}
	for _, required := range []string{"auth.setup", "asset.import", "plan.generate", "approval.decide", "task.execute", "report.export"} {
		if !seen[required] {
			t.Errorf("missing audit action %s", required)
		}
	}

	status, data = admin.request(t, "POST", "/projects", map[string]any{"name": "CSRF"}, map[string]string{
		"Origin": "https://evil.example",
	})
	if status != 403 {
		t.Fatalf("cross-origin cookie request should be rejected: %d %s", status, data)
	}

	status, _ = admin.request(t, "POST", "/auth/logout", nil, nil)
	if status != 204 {
		t.Fatalf("logout: %d", status)
	}
	status, _ = admin.request(t, "GET", "/auth/me", nil, nil)
	if status != 401 {
		t.Fatalf("session should be revoked, got %d", status)
	}
}
