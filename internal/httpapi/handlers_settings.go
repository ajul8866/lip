package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/report"
	"github.com/lip-project/lip/internal/store"
	"github.com/lip-project/lip/internal/toolcatalog"
)

func (s *Server) handleListSettings(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Q().ListSettings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Setting{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type settingUpdate struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type settingsUpdateRequest struct {
	Items []settingUpdate `json:"items"`
}

var localeSettingKey = "i18n." + "default" + "_" + "locale"

var editableSettings = map[string]bool{
	"app.name":          true,
	localeSettingKey:    true,
	"report.disclaimer": true,
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsUpdateRequest
	if !decodeJSON(w, r, &req, 32<<10) {
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 20 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "items must contain 1-20 settings")
		return
	}
	for _, item := range req.Items {
		if !editableSettings[item.Key] {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "setting is not editable: "+item.Key)
			return
		}
		if err := validateSettingValue(item.Key, item.Value); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, err.Error())
			return
		}
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	keys := make([]string, 0, len(req.Items))
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		for _, item := range req.Items {
			if _, err := q.UpsertSetting(r.Context(), item.Key, item.Value, &actx.User.ID); err != nil {
				return err
			}
			keys = append(keys, item.Key)
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "settings.update",
			TargetType: "settings", TargetID: "global", Result: "success", IP: ip,
			Metadata: map[string]any{"keys": keys},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items, err := s.store.Q().ListSettings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func validateSettingValue(key string, raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > 4096 {
		return errSettingValue
	}
	switch key {
	case "app.name":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return errSettingValue
		}
		v = strings.TrimSpace(v)
		if v == "" || len(v) > 80 {
			return errSettingValue
		}
	case localeSettingKey:
		var v string
		if err := json.Unmarshal(raw, &v); err != nil || !store.ValidLocale(v) {
			return errSettingValue
		}
	case "report.disclaimer":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil || len(v) > 2000 {
			return errSettingValue
		}
	}
	return nil
}

var errSettingValue = errors.New("invalid setting value")

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	result := strings.TrimSpace(r.URL.Query().Get("result"))
	if result != "" {
		valid := false
		for _, v := range store.AuditResults {
			if v == result {
				valid = true
				break
			}
		}
		if !valid {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid result filter")
			return
		}
	}
	since, err := parseOptionalTime(r.URL.Query().Get("since"))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "since must be RFC3339")
		return
	}
	until, err := parseOptionalTime(r.URL.Query().Get("until"))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "until must be RFC3339")
		return
	}
	limit, offset := parsePage(r)
	items, total, err := s.store.Q().ListAudit(r.Context(), store.AuditFilter{
		ActorID: strings.TrimSpace(r.URL.Query().Get("actorId")),
		Action:  strings.TrimSpace(r.URL.Query().Get("action")),
		Result:  result,
		Since:   since, Until: until, Limit: limit, Offset: offset,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	dashboard, err := s.store.Q().Dashboard(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dashboard":          dashboard,
		"safeLocalOnly":      s.cfg.SafeLocalOnly,
		"maxConcurrentTasks": s.cfg.MaxConcurrent,
		"autoRun":            s.cfg.AutoRun,
	})
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": toolcatalog.All()})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	project, err := s.store.Q().GetProject(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	assets, err := s.store.Q().ListAssetsForReport(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	findings, err := s.store.Q().ListFindingsForReport(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tasks, _, err := s.store.Q().ListTasks(r.Context(), projectID, store.TaskFilter{Limit: 10000})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	approvals, _, err := s.store.Q().ListApprovals(r.Context(), projectID, "", 10000, 0)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	var buf bytes.Buffer
	if err := report.Render(&buf, report.Data{
		AppName:       s.settingString(r.Context(), "app.name", "LIP"),
		Project:       project,
		Assets:        assets,
		Findings:      findings,
		Tasks:         tasks,
		Approvals:     approvals,
		GeneratedAt:   time.Now().UTC(),
		GeneratedBy:   actx.User.DisplayName + " <" + actx.User.Email + ">",
		SafeLocalOnly: s.cfg.SafeLocalOnly,
		Disclaimer:    s.settingString(r.Context(), "report.disclaimer", ""),
		MaxConcurrent: s.cfg.MaxConcurrent,
	}); err != nil {
		s.log.Error("render report failed", "project_id", projectID, "error", err)
		writeErr(w, http.StatusInternalServerError, codeInternal, "report generation failed")
		return
	}
	if err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "report.export",
			TargetType: "project", TargetID: projectID, Result: "success", IP: ip,
			Metadata: map[string]any{"findings": len(findings), "assets": len(assets)},
		})
	}); err != nil {
		s.fail(w, r, err)
		return
	}
	filename := reportFilename(project, time.Now().UTC())
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func reportFilename(project store.Project, at time.Time) string {
	var b strings.Builder
	for _, r := range strings.ToLower(project.Name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteByte('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "project"
	}
	return slug + "-report-" + at.Format("2006-01-02") + ".md"
}
