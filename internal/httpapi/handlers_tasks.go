package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/store"
	"github.com/lip-project/lip/internal/toolcatalog"
)

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !store.ValidTaskStatus(status) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid status filter")
		return
	}
	limit, offset := parsePage(r)
	items, total, err := s.store.Q().ListTasks(r.Context(), projectID, store.TaskFilter{
		Status: status, Limit: limit, Offset: offset,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Task{}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

type taskRequest struct {
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Tool           string          `json:"tool"`
	Parameters     json.RawMessage `json:"parameters"`
	ConversationID string          `json:"conversationId"`
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req taskRequest
	if !decodeJSON(w, r, &req, 32<<10) {
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
	if len(req.Description) > 2000 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "description must be at most 2000 characters")
		return
	}
	def, ok := toolcatalog.Get(req.Tool)
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "unknown tool")
		return
	}
	params, ok := normalizeParameters(req.Parameters)
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "parameters must be a JSON object under 8 KiB")
		return
	}
	var conversationID *string
	if req.ConversationID != "" {
		conversation, err := s.store.Q().GetConversation(r.Context(), req.ConversationID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if conversation.ProjectID != projectID {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "conversation does not belong to this project")
			return
		}
		conversationID = &conversation.ID
	}
	if def.Name == "http_head" {
		if err := s.validateHTTPHeadTarget(r, projectID, params); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, err.Error())
			return
		}
	}
	status := "queued"
	if def.RequiresApproval {
		status = "awaiting_approval"
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	task := store.Task{
		ID: store.NewID(), ProjectID: projectID, ConversationID: conversationID,
		Title: title, Description: strings.TrimSpace(req.Description), Tool: def.Name,
		Parameters: params, RiskLevel: def.Risk, Status: status,
		CreatedBy: &actx.User.ID, Result: []byte(`{}`),
	}
	var approval *store.Approval
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		created, err := q.CreateTask(r.Context(), task)
		if err != nil {
			return err
		}
		task = created
		if err := s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "task.create",
			TargetType: "task", TargetID: created.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"tool": created.Tool, "risk": created.RiskLevel},
		}); err != nil {
			return err
		}
		if def.RequiresApproval {
			createdApproval, err := q.CreateApproval(r.Context(), store.Approval{
				ID: store.NewID(), ProjectID: projectID, TaskID: created.ID,
				Action: "task.execute", Reason: "Tool requires human approval before execution.",
				RequestedBy: &actx.User.ID, ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
			})
			if err != nil {
				return err
			}
			approval = &createdApproval
			return s.audit.Record(r.Context(), q, audit.Entry{
				ActorID: actorID, ActorEmail: email, Action: "approval.create",
				TargetType: "approval", TargetID: createdApproval.ID, Result: "success", IP: ip,
				Metadata: map[string]any{"taskId": created.ID},
			})
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := map[string]any{"task": task}
	if approval != nil {
		out["approval"] = approval
	}
	writeJSON(w, http.StatusCreated, out)
}

func normalizeParameters(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`), true
	}
	if len(raw) > 8192 {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, false
	}
	return raw, true
}

func (s *Server) validateHTTPHeadTarget(r *http.Request, projectID string, params json.RawMessage) error {
	var p struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.URL == "" {
		return errors.New("http_head requires a url parameter")
	}
	u, err := url.Parse(p.URL)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("http_head url must be a valid http(s) URL without credentials")
	}
	declared, err := s.store.Q().IsDeclaredAsset(r.Context(), projectID, "url", u.String())
	if err != nil {
		return err
	}
	if !declared {
		return errors.New("http_head target must be a declared URL asset in this project")
	}
	return nil
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.store.Q().GetTask(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

type taskUpdateRequest struct {
	Title       *string         `json:"title"`
	Description *string         `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	var req taskUpdateRequest
	if !decodeJSON(w, r, &req, 32<<10) {
		return
	}
	if req.Title == nil && req.Description == nil && len(req.Parameters) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "no supported fields supplied")
		return
	}
	id := r.PathValue("id")
	existing, err := s.store.Q().GetTask(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if existing.Status != "queued" && existing.Status != "awaiting_approval" && existing.Status != "blocked" {
		writeErr(w, http.StatusConflict, codeConflict, "task can no longer be edited")
		return
	}
	if len(req.Parameters) != 0 && existing.RiskLevel != "passive" {
		writeErr(w, http.StatusConflict, codeConflict, "parameters are immutable for active and destructive tasks")
		return
	}
	update := store.TaskUpdate{}
	if req.Title != nil {
		title, ok := validateName(*req.Title, 200)
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "title must be 1-200 characters")
			return
		}
		update.Title = &title
	}
	if req.Description != nil {
		if len(*req.Description) > 2000 {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "description must be at most 2000 characters")
			return
		}
		desc := strings.TrimSpace(*req.Description)
		update.Description = &desc
	}
	if len(req.Parameters) != 0 {
		params, ok := normalizeParameters(req.Parameters)
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "parameters must be a JSON object under 8 KiB")
			return
		}
		if existing.Tool == "http_head" {
			if err := s.validateHTTPHeadTarget(r, existing.ProjectID, params); err != nil {
				writeErr(w, http.StatusUnprocessableEntity, codeValidation, err.Error())
				return
			}
		}
		update.Parameters = params
	}
	actorID, email, ip := s.actor(r)
	var updated store.Task
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		var err error
		updated, err = q.UpdateTask(r.Context(), id, update)
		if err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "task.update",
			TargetType: "task", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": updated})
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	var cancelled store.Task
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		task, err := q.GetTaskForUpdate(r.Context(), id)
		if err != nil {
			return err
		}
		if task.Status == "running" {
			return errTaskRunning
		}
		if task.Status == "succeeded" || task.Status == "failed" || task.Status == "cancelled" {
			return errTaskTerminal
		}
		cancelled, err = q.CancelTask(r.Context(), id)
		if err != nil {
			return err
		}
		if pending, err := q.GetPendingApprovalForTask(r.Context(), id); err == nil {
			if _, err := q.DecideApproval(r.Context(), pending.ID, "denied", actx.User.ID, "task cancelled by requester"); err != nil {
				return err
			}
			if err := s.audit.Record(r.Context(), q, audit.Entry{
				ActorID: actorID, ActorEmail: email, Action: "approval.decide",
				TargetType: "approval", TargetID: pending.ID, Result: "success", IP: ip,
				Metadata: map[string]any{"decision": "denied", "taskId": id, "reason": "task cancelled"},
			}); err != nil {
				return err
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "task.cancel",
			TargetType: "task", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		switch {
		case errors.Is(err, errTaskRunning):
			writeErr(w, http.StatusConflict, codeConflict, "running tasks cannot be cancelled")
		case errors.Is(err, errTaskTerminal):
			writeErr(w, http.StatusConflict, codeConflict, "task is already terminal")
		default:
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": cancelled})
}

var (
	errTaskTerminal = errors.New("task terminal")
	errTaskRunning  = errors.New("task running")
)

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actorID, email, ip := s.actor(r)
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.DeleteTask(r.Context(), id); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "task.delete",
			TargetType: "task", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !validApprovalStatus(status) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid status filter")
		return
	}
	limit, offset := parsePage(r)
	items, total, err := s.store.Q().ListApprovals(r.Context(), projectID, status, limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Approval{}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

func validApprovalStatus(status string) bool {
	for _, v := range store.ApprovalStatus {
		if v == status {
			return true
		}
	}
	return false
}

type decideApprovalRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

var (
	errApprovalNotPending = errors.New("approval not pending")
	errSelfApproval       = errors.New("self approval")
)

func (s *Server) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	var req decideApprovalRequest
	if !decodeJSON(w, r, &req, 8<<10) {
		return
	}
	decision := strings.ToLower(strings.TrimSpace(req.Decision))
	if decision != "approve" && decision != "deny" {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "decision must be approve or deny")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if decision == "deny" && len(reason) < 3 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "a reason is required when denying")
		return
	}
	if len(reason) > 500 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "reason must be at most 500 characters")
		return
	}

	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	approvalID := r.PathValue("id")
	var approval store.Approval
	var task store.Task
	expired := false

	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		var err error
		approval, err = q.GetApprovalForUpdate(r.Context(), approvalID)
		if err != nil {
			return err
		}
		if approval.Status != "pending" {
			return errApprovalNotPending
		}
		task, err = q.GetTaskForUpdate(r.Context(), approval.TaskID)
		if err != nil {
			return err
		}
		if task.Status != "awaiting_approval" {
			return errTaskTerminal
		}
		if approval.ExpiresAt.Before(time.Now().UTC()) {
			if _, err := q.DecideApproval(r.Context(), approval.ID, "expired", actx.User.ID, "expired before decision"); err != nil {
				return err
			}
			if _, err := q.BlockTask(r.Context(), task.ID, "approval expired without a decision"); err != nil {
				return err
			}
			if err := s.audit.Record(r.Context(), q, audit.Entry{
				ActorID: actorID, ActorEmail: email, Action: "approval.expire",
				TargetType: "approval", TargetID: approval.ID, Result: "success", IP: ip,
				Metadata: map[string]any{"taskId": task.ID},
			}); err != nil {
				return err
			}
			expired = true
			return nil
		}
		if approval.RequestedBy != nil && *approval.RequestedBy == actx.User.ID && actx.User.Role != "admin" {
			return errSelfApproval
		}
		if decision == "approve" {
			if _, err := q.DecideApproval(r.Context(), approval.ID, "approved", actx.User.ID, reason); err != nil {
				return err
			}
			task, err = q.ApproveTask(r.Context(), task.ID, actx.User.ID)
			if err != nil {
				return err
			}
			if err := s.audit.Record(r.Context(), q, audit.Entry{
				ActorID: actorID, ActorEmail: email, Action: "approval.decide",
				TargetType: "approval", TargetID: approval.ID, Result: "success", IP: ip,
				Metadata: map[string]any{"decision": "approved", "taskId": task.ID},
			}); err != nil {
				return err
			}
			return s.audit.Record(r.Context(), q, audit.Entry{
				ActorID: actorID, ActorEmail: email, Action: "task.approve",
				TargetType: "task", TargetID: task.ID, Result: "success", IP: ip,
			})
		}
		if _, err := q.DecideApproval(r.Context(), approval.ID, "denied", actx.User.ID, reason); err != nil {
			return err
		}
		task, err = q.CancelTask(r.Context(), task.ID)
		if err != nil {
			return err
		}
		if err := s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "approval.decide",
			TargetType: "approval", TargetID: approval.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"decision": "denied", "taskId": task.ID, "reason": reason},
		}); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "task.deny",
			TargetType: "task", TargetID: task.ID, Result: "success", IP: ip,
		})
	})
	if err != nil {
		switch {
		case errors.Is(err, errApprovalNotPending):
			writeErr(w, http.StatusConflict, codeConflict, "approval is no longer pending")
		case errors.Is(err, errSelfApproval):
			writeErr(w, http.StatusForbidden, codeForbidden, "requesters cannot approve their own active task")
		case errors.Is(err, errTaskTerminal):
			writeErr(w, http.StatusConflict, codeConflict, "task is not awaiting approval")
		default:
			s.fail(w, r, err)
		}
		return
	}
	if expired {
		writeErr(w, http.StatusConflict, codeConflict, "approval expired")
		return
	}
	approval, _ = s.store.Q().GetApproval(r.Context(), approval.ID)
	writeJSON(w, http.StatusOK, map[string]any{"approval": approval, "task": task})
}
