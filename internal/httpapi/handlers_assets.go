package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lip-project/lip/internal/assets"
	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/egress"
	"github.com/lip-project/lip/internal/store"
)

func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind != "" && !store.ValidAssetKind(kind) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid asset kind filter")
		return
	}
	limit, offset := parsePage(r)
	items, total, err := s.store.Q().ListAssets(r.Context(), projectID, kind, limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Asset{}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

type assetRequest struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Source     string `json:"source"`
	Attributes any    `json:"attributes"`
}

func (s *Server) handleCreateAsset(w http.ResponseWriter, r *http.Request) {
	var req assetRequest
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	value, ok := assets.Normalize(kind, req.Value)
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid asset kind or value")
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "manual"
	}
	if len(source) > 100 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "source must be at most 100 characters")
		return
	}
	attrs, err := jsonRawObject(req.Attributes, 8192)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "attributes must be a JSON object under 8 KiB")
		return
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	asset := store.Asset{
		ID: store.NewID(), ProjectID: projectID, Kind: kind, Value: value,
		Source: source, Attributes: attrs, CreatedBy: &actx.User.ID,
	}
	var inserted bool
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		var err error
		asset, inserted, err = q.UpsertAsset(r.Context(), asset)
		if err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "asset.create",
			TargetType: "asset", TargetID: asset.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"kind": asset.Kind, "inserted": inserted},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if inserted {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"asset": asset, "inserted": inserted})
}

type importRequest struct {
	Source string   `json:"source"`
	Values []string `json:"values"`
	Path   string   `json:"path"`
	URL    string   `json:"url"`
}

func (s *Server) handleImportAssets(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if !decodeJSON(w, r, &req, 16<<20) {
		return
	}
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	source := strings.ToLower(strings.TrimSpace(req.Source))
	if source == "" {
		source = "inline"
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)

	var reader io.Reader
	switch source {
	case "inline":
		if len(req.Values) == 0 {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "values is required for inline import")
			return
		}
		if len(req.Values) > assets.MaxItems {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "too many inline assets")
			return
		}
		reader = strings.NewReader(strings.Join(req.Values, "\n"))
	case "file":
		if req.Path == "" {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "path is required for file import")
			return
		}
		data, err := assets.ReadLocal(s.cfg.ImportRoot, req.Path)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, err.Error())
			return
		}
		reader = bytes.NewReader(data)
	case "url":
		if req.URL == "" {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "url is required for url import")
			return
		}
		if s.cfg.SafeLocalOnly {
			s.recordAudit(r.Context(), audit.Entry{
				ActorID: actorID, ActorEmail: email, Action: "asset.import",
				TargetType: "project", TargetID: projectID, Result: "denied", IP: ip,
				Metadata: map[string]any{"source": "url", "reason": "safe local-only mode"},
			})
			writeErr(w, http.StatusForbidden, codeSafeMode, "safe local-only mode blocks remote asset imports")
			return
		}
		u, err := s.guard.CheckURL(r.Context(), req.URL)
		if err != nil {
			s.recordAudit(r.Context(), audit.Entry{
				ActorID: actorID, ActorEmail: email, Action: "asset.import",
				TargetType: "project", TargetID: projectID, Result: "denied", IP: ip,
				Metadata: map[string]any{"source": "url", "reason": "egress policy"},
			})
			writeErr(w, http.StatusForbidden, codeEgress, egress.ErrorMessage(err))
			return
		}
		httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid import URL")
			return
		}
		httpReq.Header.Set("User-Agent", "lip-import")
		resp, err := s.guard.Client(30 * time.Second).Do(httpReq)
		if err != nil {
			writeErr(w, http.StatusBadGateway, codeProvider, "remote import failed")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			writeErr(w, http.StatusBadGateway, codeProvider, "remote import returned a non-2xx status")
			return
		}
		reader = io.LimitReader(resp.Body, assets.MaxFileSize+1)
	default:
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "source must be inline, file, or url")
		return
	}

	data, err := io.ReadAll(io.LimitReader(reader, assets.MaxFileSize+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, codeValidation, "could not read asset source")
		return
	}
	if len(data) > assets.MaxFileSize {
		writeErr(w, http.StatusRequestEntityTooLarge, codePayloadTooBig, "asset source exceeds 10 MiB")
		return
	}
	parsed, err := assets.Parse(bytes.NewReader(data))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, err.Error())
		return
	}
	inserted, updated := 0, 0
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		for _, item := range parsed.Items {
			_, isNew, err := q.UpsertAsset(r.Context(), store.Asset{
				ID: store.NewID(), ProjectID: projectID, Kind: item.Kind, Value: item.Value,
				Source: source, CreatedBy: &actx.User.ID,
			})
			if err != nil {
				return err
			}
			if isNew {
				inserted++
			} else {
				updated++
			}
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "asset.import",
			TargetType: "project", TargetID: projectID, Result: "success", IP: ip,
			Metadata: map[string]any{
				"source": source, "inserted": inserted, "updated": updated,
				"skipped": parsed.Skipped, "total": len(parsed.Items),
			},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"inserted": inserted, "updated": updated,
		"skipped": parsed.Skipped, "total": len(parsed.Items), "source": source,
	})
}

func (s *Server) handleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actorID, email, ip := s.actor(r)
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.DeleteAsset(r.Context(), id); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "asset.delete",
			TargetType: "asset", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func jsonRawObject(v any, max int) ([]byte, error) {
	if v == nil {
		return []byte(`{}`), nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(raw) > max {
		return nil, errors.New("object too large")
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, errors.New("not an object")
	}
	return raw, nil
}
