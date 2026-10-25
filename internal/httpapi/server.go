package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/config"
	"github.com/lip-project/lip/internal/egress"
	"github.com/lip-project/lip/internal/i18n"
	"github.com/lip-project/lip/internal/llm"
	"github.com/lip-project/lip/internal/ratelimit"
	"github.com/lip-project/lip/internal/store"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	guard   *egress.Guard
	llm     *llm.Registry
	audit   *audit.Recorder
	bundle  *i18n.Bundle
	log     *slog.Logger
	ui      http.Handler
	started time.Time

	loginLimiter *ratelimit.Limiter
	emailLimiter *ratelimit.Limiter
	setupLimiter *ratelimit.Limiter

	handler http.Handler
}

func New(
	cfg config.Config,
	st *store.Store,
	guard *egress.Guard,
	registry *llm.Registry,
	rec *audit.Recorder,
	bundle *i18n.Bundle,
	log *slog.Logger,
	ui http.Handler,
) *Server {
	s := &Server{
		cfg:          cfg,
		store:        st,
		guard:        guard,
		llm:          registry,
		audit:        rec,
		bundle:       bundle,
		log:          log,
		ui:           ui,
		started:      time.Now().UTC(),
		loginLimiter: ratelimit.New(5, 15*time.Minute),
		emailLimiter: ratelimit.New(20, 15*time.Minute),
		setupLimiter: ratelimit.New(5, 15*time.Minute),
	}
	s.handler = s.buildHandler()
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) buildHandler() http.Handler {
	api := http.NewServeMux()
	s.routes(api)
	api.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, codeNotFound, "endpoint not found")
	}))

	root := http.NewServeMux()
	root.Handle("/api/", api)
	root.Handle("/api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, codeNotFound, "endpoint not found")
	}))
	if s.ui != nil {
		root.Handle("/", s.ui)
	} else {
		root.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "UI is not embedded in this build", http.StatusServiceUnavailable)
		}))
	}

	var h http.Handler = root
	h = s.securityHeaders(h)
	h = s.recoverer(h)
	h = s.accessLog(h)
	h = s.requestID(h)
	return h
}

func (s *Server) routes(mux *http.ServeMux) {
	s.public(mux, "GET /api/v1/healthz", s.handleHealth)
	s.public(mux, "GET /api/v1/readyz", s.handleReady)
	s.public(mux, "GET /api/v1/system/status", s.handleSystemStatus)
	s.public(mux, "GET /api/v1/setup/status", s.handleSetupStatus)
	s.public(mux, "POST /api/v1/setup", s.handleSetup)
	s.public(mux, "POST /api/v1/auth/login", s.handleLogin)
	s.public(mux, "GET /api/v1/i18n/{locale}", s.handleI18n)

	s.protected(mux, "GET /api/v1/auth/me", s.handleMe)
	s.protected(mux, "PATCH /api/v1/auth/me", s.handleUpdateMe)
	s.protected(mux, "POST /api/v1/auth/logout", s.handleLogout)
	s.protected(mux, "POST /api/v1/auth/change-password", s.handleChangePassword)
	s.protected(mux, "GET /api/v1/sessions", s.handleListSessions)
	s.protected(mux, "DELETE /api/v1/sessions/{id}", s.handleRevokeSession)

	s.protected(mux, "GET /api/v1/users", s.handleListUsers, "admin")
	s.protected(mux, "POST /api/v1/users", s.handleCreateUser, "admin")
	s.protected(mux, "PATCH /api/v1/users/{id}", s.handleUpdateUser, "admin")
	s.protected(mux, "POST /api/v1/users/{id}/reset-password", s.handleResetUserPassword, "admin")

	s.protected(mux, "GET /api/v1/projects", s.handleListProjects)
	s.protected(mux, "POST /api/v1/projects", s.handleCreateProject, "analyst")
	s.protected(mux, "GET /api/v1/projects/{id}", s.handleGetProject)
	s.protected(mux, "PATCH /api/v1/projects/{id}", s.handleUpdateProject, "analyst")
	s.protected(mux, "DELETE /api/v1/projects/{id}", s.handleArchiveProject, "admin")

	s.protected(mux, "GET /api/v1/projects/{id}/conversations", s.handleListConversations)
	s.protected(mux, "POST /api/v1/projects/{id}/conversations", s.handleCreateConversation, "analyst")
	s.protected(mux, "GET /api/v1/conversations/{id}", s.handleGetConversation)
	s.protected(mux, "PATCH /api/v1/conversations/{id}", s.handleUpdateConversation, "analyst")
	s.protected(mux, "DELETE /api/v1/conversations/{id}", s.handleDeleteConversation, "analyst")
	s.protected(mux, "GET /api/v1/conversations/{id}/messages", s.handleListMessages)
	s.protected(mux, "POST /api/v1/conversations/{id}/messages", s.handleCreateMessage, "analyst")
	s.protected(mux, "POST /api/v1/conversations/{id}/plan", s.handleGeneratePlan, "analyst")

	s.protected(mux, "GET /api/v1/projects/{id}/tasks", s.handleListTasks)
	s.protected(mux, "POST /api/v1/projects/{id}/tasks", s.handleCreateTask, "analyst")
	s.protected(mux, "GET /api/v1/tasks/{id}", s.handleGetTask)
	s.protected(mux, "PATCH /api/v1/tasks/{id}", s.handleUpdateTask, "analyst")
	s.protected(mux, "DELETE /api/v1/tasks/{id}", s.handleDeleteTask, "admin")
	s.protected(mux, "POST /api/v1/tasks/{id}/cancel", s.handleCancelTask, "analyst")

	s.protected(mux, "GET /api/v1/projects/{id}/approvals", s.handleListApprovals)
	s.protected(mux, "POST /api/v1/approvals/{id}/decide", s.handleDecideApproval, "analyst")

	s.protected(mux, "GET /api/v1/projects/{id}/findings", s.handleListFindings)
	s.protected(mux, "POST /api/v1/projects/{id}/findings", s.handleCreateFinding, "analyst")
	s.protected(mux, "GET /api/v1/findings/{id}", s.handleGetFinding)
	s.protected(mux, "PATCH /api/v1/findings/{id}", s.handleUpdateFinding, "analyst")
	s.protected(mux, "DELETE /api/v1/findings/{id}", s.handleDeleteFinding, "analyst")

	s.protected(mux, "GET /api/v1/projects/{id}/assets", s.handleListAssets)
	s.protected(mux, "POST /api/v1/projects/{id}/assets", s.handleCreateAsset, "analyst")
	s.protected(mux, "POST /api/v1/projects/{id}/assets/import", s.handleImportAssets, "analyst")
	s.protected(mux, "DELETE /api/v1/assets/{id}", s.handleDeleteAsset, "analyst")

	s.protected(mux, "GET /api/v1/llm-providers", s.handleListProviders, "admin")
	s.protected(mux, "POST /api/v1/llm-providers", s.handleCreateProvider, "admin")
	s.protected(mux, "PATCH /api/v1/llm-providers/{id}", s.handleUpdateProvider, "admin")
	s.protected(mux, "DELETE /api/v1/llm-providers/{id}", s.handleDeleteProvider, "admin")
	s.protected(mux, "POST /api/v1/llm-providers/{id}/test", s.handleTestProvider, "admin")

	s.protected(mux, "GET /api/v1/settings", s.handleListSettings, "admin")
	s.protected(mux, "PATCH /api/v1/settings", s.handleUpdateSettings, "admin")
	s.protected(mux, "GET /api/v1/audit", s.handleListAudit, "admin")
	s.protected(mux, "GET /api/v1/dashboard", s.handleDashboard)
	s.protected(mux, "GET /api/v1/meta/tools", s.handleTools)
	s.protected(mux, "GET /api/v1/projects/{id}/report", s.handleReport)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeErr(w, http.StatusServiceUnavailable, codeInternal, "database not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	userCount, err := s.store.Q().CountUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, codeInternal, "database unavailable")
		return
	}
	appName := s.settingString(r.Context(), "app.name", "LIP")
	defaultLocale := s.settingString(r.Context(), "i18n.default_locale", s.cfg.DefaultLocale)
	writeJSON(w, http.StatusOK, map[string]any{
		"version":            s.cfg.Version,
		"appName":            appName,
		"startedAt":          s.started,
		"setupRequired":      userCount == 0,
		"defaultLocale":      defaultLocale,
		"supportedLocales":   s.bundle.Supported(),
		"safeLocalOnly":      s.cfg.SafeLocalOnly,
		"safeModeSource":     s.cfg.SafeModeSource,
		"egressRuleCount":    s.guard.RuleCount(),
		"maxConcurrentTasks": s.cfg.MaxConcurrent,
		"autoRun":            s.cfg.AutoRun,
	})
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.Q().CountUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, codeInternal, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setupRequired": n == 0,
		"appName":       s.settingString(r.Context(), "app.name", "LIP"),
		"safeLocalOnly": s.cfg.SafeLocalOnly,
	})
}

func (s *Server) handleI18n(w http.ResponseWriter, r *http.Request) {
	locale := r.PathValue("locale")
	if !s.bundle.Has(locale) {
		writeErr(w, http.StatusNotFound, codeNotFound, "locale not found")
		return
	}
	selected, stringsMap, _ := s.bundle.Get(locale)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{"locale": selected, "strings": stringsMap})
}

func (s *Server) settingString(ctx context.Context, key, fallback string) string {
	setting, err := s.store.Q().GetSetting(ctx, key)
	if err != nil {
		return fallback
	}
	var out string
	if json.Unmarshal(setting.Value, &out) == nil && out != "" {
		return out
	}
	return fallback
}
