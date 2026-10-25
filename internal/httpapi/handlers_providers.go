package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/egress"
	"github.com/lip-project/lip/internal/llm"
	"github.com/lip-project/lip/internal/store"
)

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Q().ListProviders(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.LLMProvider{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type providerRequest struct {
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	BaseURL   string          `json:"baseUrl"`
	Model     string          `json:"model"`
	APIKeyEnv string          `json:"apiKeyEnv"`
	Enabled   *bool           `json:"enabled"`
	IsDefault bool            `json:"isDefault"`
	Config    json.RawMessage `json:"config"`
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req providerRequest
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	cfg, ok := s.providerFromRequest(w, r, req, store.LLMProvider{Enabled: true})
	if !ok {
		return
	}
	cfg.ID = store.NewID()
	actorID, email, ip := s.actor(r)
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		if cfg.IsDefault {
			if err := q.ClearDefaultProviders(r.Context(), cfg.ID); err != nil {
				return err
			}
		}
		created, err := q.CreateProvider(r.Context(), cfg)
		if err != nil {
			return err
		}
		cfg = created
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "llm_provider.create",
			TargetType: "llm_provider", TargetID: created.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"kind": created.Kind, "model": created.Model, "default": created.IsDefault},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"provider": cfg})
}

func (s *Server) providerFromRequest(w http.ResponseWriter, r *http.Request, req providerRequest, base store.LLMProvider) (store.LLMProvider, bool) {
	base.Name = strings.TrimSpace(req.Name)
	if base.Name == "" || len(base.Name) > 80 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "name must be 1-80 characters")
		return store.LLMProvider{}, false
	}
	base.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	if !store.ValidProviderKind(base.Kind) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "kind must be ollama or openai")
		return store.LLMProvider{}, false
	}
	base.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	base.Model = strings.TrimSpace(req.Model)
	if base.Model == "" || len(base.Model) > 120 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "model must be 1-120 characters")
		return store.LLMProvider{}, false
	}
	base.APIKeyEnv = strings.TrimSpace(req.APIKeyEnv)
	base.IsDefault = req.IsDefault
	if req.Enabled != nil {
		base.Enabled = *req.Enabled
	}
	config := req.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	var obj map[string]any
	if err := json.Unmarshal(config, &obj); err != nil || obj == nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "config must be a JSON object")
		return store.LLMProvider{}, false
	}
	base.Config = config
	if err := s.llm.Validate(r.Context(), base); err != nil {
		s.writeProviderConfigError(w, err)
		return store.LLMProvider{}, false
	}
	return base, true
}

func (s *Server) writeProviderConfigError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, llm.ErrProviderDenied):
		writeErr(w, http.StatusForbidden, codeSafeMode, "provider is blocked by safe local-only mode or egress policy")
	case errors.Is(err, egress.ErrDenied):
		writeErr(w, http.StatusForbidden, codeEgress, egress.ErrorMessage(err))
	case errors.Is(err, store.ErrInvalid):
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, strings.TrimPrefix(err.Error(), "invalid: "))
	case errors.Is(err, llm.ErrNoProvider):
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "provider configuration is incomplete; check the API key environment variable")
	default:
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid provider configuration")
	}
}

type providerUpdateRequest struct {
	Name      *string         `json:"name"`
	Kind      *string         `json:"kind"`
	BaseURL   *string         `json:"baseUrl"`
	Model     *string         `json:"model"`
	APIKeyEnv *string         `json:"apiKeyEnv"`
	Enabled   *bool           `json:"enabled"`
	IsDefault *bool           `json:"isDefault"`
	Config    json.RawMessage `json:"config"`
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	var req providerUpdateRequest
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	id := r.PathValue("id")
	existing, err := s.store.Q().GetProvider(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	merged := providerRequest{
		Name: existing.Name, Kind: existing.Kind, BaseURL: existing.BaseURL,
		Model: existing.Model, APIKeyEnv: existing.APIKeyEnv,
		Enabled: &existing.Enabled, IsDefault: existing.IsDefault, Config: existing.Config,
	}
	if req.Name != nil {
		merged.Name = *req.Name
	}
	if req.Kind != nil {
		merged.Kind = *req.Kind
	}
	if req.BaseURL != nil {
		merged.BaseURL = *req.BaseURL
	}
	if req.Model != nil {
		merged.Model = *req.Model
	}
	if req.APIKeyEnv != nil {
		merged.APIKeyEnv = *req.APIKeyEnv
	}
	if req.Enabled != nil {
		merged.Enabled = req.Enabled
	}
	if req.IsDefault != nil {
		merged.IsDefault = *req.IsDefault
	}
	if len(req.Config) != 0 {
		merged.Config = req.Config
	}
	cfg, ok := s.providerFromRequest(w, r, merged, existing)
	if !ok {
		return
	}
	actorID, email, ip := s.actor(r)
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		if cfg.IsDefault {
			if err := q.ClearDefaultProviders(r.Context(), id); err != nil {
				return err
			}
		}
		updated, err := q.UpdateProvider(r.Context(), id, store.ProviderUpdate{
			Name: &cfg.Name, Kind: &cfg.Kind, BaseURL: &cfg.BaseURL, Model: &cfg.Model,
			APIKeyEnv: &cfg.APIKeyEnv, Enabled: &cfg.Enabled, IsDefault: &cfg.IsDefault,
			Config: cfg.Config,
		})
		if err != nil {
			return err
		}
		cfg = updated
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "llm_provider.update",
			TargetType: "llm_provider", TargetID: id, Result: "success", IP: ip,
			Metadata: map[string]any{"kind": updated.Kind, "enabled": updated.Enabled, "default": updated.IsDefault},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": cfg})
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.store.Q().GetProvider(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if existing.IsDefault {
		writeErr(w, http.StatusConflict, codeConflict, "choose another default provider before deleting this one")
		return
	}
	actorID, email, ip := s.actor(r)
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.DeleteProvider(r.Context(), id); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "llm_provider.delete",
			TargetType: "llm_provider", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actorID, email, ip := s.actor(r)
	err := s.llm.Test(r.Context(), s.store.Q(), id)
	result := "success"
	if err != nil {
		result = "failure"
	}
	s.recordAudit(r.Context(), audit.Entry{
		ActorID: actorID, ActorEmail: email, Action: "llm_provider.test",
		TargetType: "llm_provider", TargetID: id, Result: result, IP: ip,
	})
	if err != nil {
		switch {
		case errors.Is(err, llm.ErrProviderDenied):
			writeErr(w, http.StatusForbidden, codeSafeMode, "provider is blocked by safe local-only mode")
		case errors.Is(err, egress.ErrDenied):
			writeErr(w, http.StatusForbidden, codeEgress, egress.ErrorMessage(err))
		case errors.Is(err, llm.ErrNoProvider), errors.Is(err, store.ErrNotFound):
			writeErr(w, http.StatusUnprocessableEntity, codeNoProvider, "provider is not configured or has no API key")
		default:
			writeErr(w, http.StatusBadGateway, codeProvider, "provider test failed")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
