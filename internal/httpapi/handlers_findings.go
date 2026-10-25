package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/store"
)

func (s *Server) handleListFindings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	severity := strings.TrimSpace(r.URL.Query().Get("severity"))
	if severity != "" && !store.ValidSeverity(severity) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid severity filter")
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !store.ValidFindingStatus(status) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid status filter")
		return
	}
	limit, offset := parsePage(r)
	items, total, err := s.store.Q().ListFindings(r.Context(), projectID, store.FindingFilter{
		Severity: severity, Status: status, Limit: limit, Offset: offset,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Finding{}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

type findingRequest struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Severity    string          `json:"severity"`
	Status      string          `json:"status"`
	AssetID     string          `json:"assetId"`
	TaskID      string          `json:"taskId"`
	Evidence    json.RawMessage `json:"evidence"`
}

func (s *Server) handleCreateFinding(w http.ResponseWriter, r *http.Request) {
	var req findingRequest
	if !decodeJSON(w, r, &req, 64<<10) {
		return
	}
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	title, ok := validateName(req.Title, 200)
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "title must be 1-200 characters")
		return
	}
	if len(req.Description) > 8000 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "description must be at most 8000 characters")
		return
	}
	severity := strings.ToLower(strings.TrimSpace(req.Severity))
	if severity == "" {
		severity = "info"
	}
	if !store.ValidSeverity(severity) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid severity")
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = "open"
	}
	if !store.ValidFindingStatus(status) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid status")
		return
	}
	evidence, ok := normalizeParameters(req.Evidence)
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "evidence must be a JSON object under 8 KiB")
		return
	}
	var assetID, taskID *string
	if req.AssetID != "" {
		asset, err := s.store.Q().GetAsset(r.Context(), req.AssetID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if asset.ProjectID != projectID {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "asset does not belong to this project")
			return
		}
		assetID = &asset.ID
	}
	if req.TaskID != "" {
		task, err := s.store.Q().GetTask(r.Context(), req.TaskID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if task.ProjectID != projectID {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "task does not belong to this project")
			return
		}
		taskID = &task.ID
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	finding := store.Finding{
		ID: store.NewID(), ProjectID: projectID, TaskID: taskID, AssetID: assetID,
		Title: title, Description: strings.TrimSpace(req.Description), Severity: severity,
		Status: status, Source: "manual", Evidence: evidence, CreatedBy: &actx.User.ID,
	}
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		created, err := q.CreateFinding(r.Context(), finding)
		if err != nil {
			return err
		}
		finding = created
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "finding.create",
			TargetType: "finding", TargetID: created.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"source": "manual", "severity": created.Severity},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"finding": finding})
}

func (s *Server) handleGetFinding(w http.ResponseWriter, r *http.Request) {
	finding, err := s.store.Q().GetFinding(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"finding": finding})
}

type findingUpdateRequest struct {
	Title       *string         `json:"title"`
	Description *string         `json:"description"`
	Severity    *string         `json:"severity"`
	Status      *string         `json:"status"`
	Evidence    json.RawMessage `json:"evidence"`
}

func (s *Server) handleUpdateFinding(w http.ResponseWriter, r *http.Request) {
	var req findingUpdateRequest
	if !decodeJSON(w, r, &req, 64<<10) {
		return
	}
	if req.Title == nil && req.Description == nil && req.Severity == nil && req.Status == nil && len(req.Evidence) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "no supported fields supplied")
		return
	}
	update := store.FindingUpdate{}
	if req.Title != nil {
		title, ok := validateName(*req.Title, 200)
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "title must be 1-200 characters")
			return
		}
		update.Title = &title
	}
	if req.Description != nil {
		if len(*req.Description) > 8000 {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "description must be at most 8000 characters")
			return
		}
		desc := strings.TrimSpace(*req.Description)
		update.Description = &desc
	}
	if req.Severity != nil {
		severity := strings.ToLower(strings.TrimSpace(*req.Severity))
		if !store.ValidSeverity(severity) {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid severity")
			return
		}
		update.Severity = &severity
	}
	if req.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*req.Status))
		if !store.ValidFindingStatus(status) {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid status")
			return
		}
		update.Status = &status
	}
	if len(req.Evidence) != 0 {
		evidence, ok := normalizeParameters(req.Evidence)
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "evidence must be a JSON object under 8 KiB")
			return
		}
		update.Evidence = evidence
	}
	id := r.PathValue("id")
	actorID, email, ip := s.actor(r)
	var updated store.Finding
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		var err error
		updated, err = q.UpdateFinding(r.Context(), id, update)
		if err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "finding.update",
			TargetType: "finding", TargetID: id, Result: "success", IP: ip,
			Metadata: map[string]any{"severity": updated.Severity, "status": updated.Status},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"finding": updated})
}

func (s *Server) handleDeleteFinding(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actorID, email, ip := s.actor(r)
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.DeleteFinding(r.Context(), id); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "finding.delete",
			TargetType: "finding", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
