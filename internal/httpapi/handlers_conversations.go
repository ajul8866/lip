package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/egress"
	"github.com/lip-project/lip/internal/llm"
	"github.com/lip-project/lip/internal/store"
	"github.com/lip-project/lip/internal/toolcatalog"
)

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	limit, offset := parsePage(r)
	items, total, err := s.store.Q().ListConversations(r.Context(), projectID, limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Conversation{}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

type conversationRequest struct {
	Title string `json:"title"`
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	var req conversationRequest
	if !decodeJSON(w, r, &req, 8<<10) {
		return
	}
	projectID := r.PathValue("id")
	if _, err := s.store.Q().GetProject(r.Context(), projectID); err != nil {
		s.fail(w, r, err)
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "New conversation"
	}
	if len(title) > 200 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "title must be at most 200 characters")
		return
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	conversation := store.Conversation{
		ID: store.NewID(), ProjectID: projectID, Title: title, CreatedBy: &actx.User.ID,
	}
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		created, err := q.CreateConversation(r.Context(), conversation)
		if err != nil {
			return err
		}
		conversation = created
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "conversation.create",
			TargetType: "conversation", TargetID: created.ID, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"conversation": conversation})
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.store.Q().GetConversation(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": conversation})
}

type conversationUpdateRequest struct {
	Title *string `json:"title"`
}

func (s *Server) handleUpdateConversation(w http.ResponseWriter, r *http.Request) {
	var req conversationUpdateRequest
	if !decodeJSON(w, r, &req, 8<<10) {
		return
	}
	if req.Title == nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "title is required")
		return
	}
	title := strings.TrimSpace(*req.Title)
	if title == "" || len(title) > 200 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "title must be 1-200 characters")
		return
	}
	id := r.PathValue("id")
	actorID, email, ip := s.actor(r)
	var updated store.Conversation
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		var err error
		updated, err = q.UpdateConversationTitle(r.Context(), id, title)
		if err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "conversation.update",
			TargetType: "conversation", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": updated})
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actorID, email, ip := s.actor(r)
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.DeleteConversation(r.Context(), id); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "conversation.delete",
			TargetType: "conversation", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.Q().GetConversation(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	limit, offset := parsePage(r)
	items, total, err := s.store.Q().ListMessages(r.Context(), id, limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Message{}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

type messageRequest struct {
	Content string `json:"content"`
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	var req messageRequest
	if !decodeJSON(w, r, &req, 64<<10) {
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || len(content) > 20000 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "content must be 1-20000 characters")
		return
	}
	conversationID := r.PathValue("id")
	if _, err := s.store.Q().GetConversation(r.Context(), conversationID); err != nil {
		s.fail(w, r, err)
		return
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	msg := store.Message{
		ID: store.NewID(), ConversationID: conversationID, Role: "user",
		Content: content, CreatedBy: &actx.User.ID, Metadata: []byte(`{}`),
	}
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		created, err := q.CreateMessage(r.Context(), msg)
		if err != nil {
			return err
		}
		msg = created
		if err := q.TouchConversation(r.Context(), conversationID); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "message.create",
			TargetType: "conversation", TargetID: conversationID, Result: "success", IP: ip,
			Metadata: map[string]any{"messageId": created.ID, "role": "user"},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"message": msg})
}

type planRequest struct {
	ProviderID string `json:"providerId"`
}

func (s *Server) handleGeneratePlan(w http.ResponseWriter, r *http.Request) {
	var req planRequest
	if r.ContentLength != 0 {
		if !decodeJSON(w, r, &req, 8<<10) {
			return
		}
	}
	conversationID := r.PathValue("id")
	conversation, err := s.store.Q().GetConversation(r.Context(), conversationID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	project, err := s.store.Q().GetProject(r.Context(), conversation.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	assets, _, err := s.store.Q().ListAssets(r.Context(), project.ID, "", 500, 0)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	total, err := s.store.Q().CountMessages(r.Context(), conversationID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	offset := int(total) - 30
	if offset < 0 {
		offset = 0
	}
	messages, _, err := s.store.Q().ListMessages(r.Context(), conversationID, 30, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	plan, provider, err := s.llm.Plan(r.Context(), s.store.Q(), req.ProviderID, project, assets, messages)
	if err != nil {
		actorID, email, ip := s.actor(r)
		s.recordAudit(r.Context(), audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "plan.generate",
			TargetType: "conversation", TargetID: conversationID, Result: "failure", IP: ip,
			Metadata: map[string]any{"reason": publicLLMError(err)},
		})
		s.writeLLMError(w, err)
		return
	}
	// Reject a plan that targets an undeclared URL before any task is stored.
	for _, t := range plan {
		if t.Tool != "http_head" {
			continue
		}
		var params struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(t.Parameters, &params); err != nil || params.URL == "" {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "plan contains an invalid http_head target")
			return
		}
		u, err := url.Parse(params.URL)
		if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "plan contains an invalid http_head target")
			return
		}
		declared, err := s.store.Q().IsDeclaredAsset(r.Context(), project.ID, "url", u.String())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !declared {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "plan targets a URL that is not a declared asset")
			return
		}
	}

	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	var tasks []store.Task
	var assistant store.Message
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		for _, planned := range plan {
			def, ok := toolcatalog.Get(planned.Tool)
			if !ok {
				return errors.New("plan referenced an unknown tool")
			}
			status := "queued"
			if def.RequiresApproval {
				status = "awaiting_approval"
			}
			task := store.Task{
				ID: store.NewID(), ProjectID: project.ID, ConversationID: &conversation.ID,
				Title: planned.Title, Description: planned.Description, Tool: planned.Tool,
				Parameters: planned.Parameters, RiskLevel: def.Risk, Status: status,
				CreatedBy: &actx.User.ID, Result: []byte(`{}`),
			}
			created, err := q.CreateTask(r.Context(), task)
			if err != nil {
				return err
			}
			tasks = append(tasks, created)
			if err := s.audit.Record(r.Context(), q, audit.Entry{
				ActorID: actorID, ActorEmail: email, Action: "task.create",
				TargetType: "task", TargetID: created.ID, Result: "success", IP: ip,
				Metadata: map[string]any{"tool": created.Tool, "risk": created.RiskLevel, "source": "plan"},
			}); err != nil {
				return err
			}
			if def.RequiresApproval {
				approval := store.Approval{
					ID: store.NewID(), ProjectID: project.ID, TaskID: created.ID,
					Action: "task.execute", Reason: "Tool requires human approval before execution.",
					RequestedBy: &actx.User.ID, ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
				}
				if _, err := q.CreateApproval(r.Context(), approval); err != nil {
					return err
				}
				if err := s.audit.Record(r.Context(), q, audit.Entry{
					ActorID: actorID, ActorEmail: email, Action: "approval.create",
					TargetType: "approval", TargetID: approval.ID, Result: "success", IP: ip,
					Metadata: map[string]any{"taskId": created.ID},
				}); err != nil {
					return err
				}
			}
		}
		meta := []byte(`{"kind":"plan"}`)
		assistant = store.Message{
			ID: store.NewID(), ConversationID: conversation.ID, Role: "assistant",
			Content:  "Plan generated with " + strconv.Itoa(len(tasks)) + " task(s). Active tasks require approval.",
			Metadata: meta,
		}
		createdMsg, err := q.CreateMessage(r.Context(), assistant)
		if err != nil {
			return err
		}
		assistant = createdMsg
		if err := q.TouchConversation(r.Context(), conversation.ID); err != nil {
			return err
		}
		if err := s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "plan.generate",
			TargetType: "conversation", TargetID: conversation.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"tasks": len(tasks), "provider": provider.Name, "model": provider.Model},
		}); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "message.create",
			TargetType: "conversation", TargetID: conversation.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"messageId": createdMsg.ID, "role": "assistant", "kind": "plan"},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"tasks": tasks, "message": assistant,
		"provider": map[string]string{"name": provider.Name, "model": provider.Model},
	})
}

func (s *Server) writeLLMError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, llm.ErrNoProvider):
		writeErr(w, http.StatusConflict, codeNoProvider, "no enabled LLM provider is configured")
	case errors.Is(err, llm.ErrProviderDenied):
		writeErr(w, http.StatusForbidden, codeSafeMode, "LLM provider is blocked by safe local-only mode")
	case errors.Is(err, egress.ErrDenied):
		writeErr(w, http.StatusForbidden, codeEgress, egress.ErrorMessage(err))
	case errors.Is(err, llm.ErrBadOutput):
		writeErr(w, http.StatusBadGateway, codeProvider, "provider returned invalid structured output")
	default:
		writeErr(w, http.StatusBadGateway, codeProvider, publicLLMError(err))
	}
}

func publicLLMError(err error) string {
	switch {
	case errors.Is(err, llm.ErrNoProvider):
		return "no enabled LLM provider"
	case errors.Is(err, llm.ErrProviderDenied):
		return "provider blocked by safe mode"
	case errors.Is(err, llm.ErrBadOutput):
		return "invalid model output"
	case errors.Is(err, egress.ErrDenied):
		return "egress blocked"
	default:
		return "provider request failed"
	}
}
