package httpapi

import (
	"net/http"
	"strings"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/store"
)

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, total, err := s.store.Q().ListProjects(r.Context(), limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Project{}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

type projectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req projectRequest
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	name, ok := validateName(req.Name, 120)
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "name must be 1-120 characters")
		return
	}
	if len(req.Description) > 4000 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "description is too long")
		return
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	project := store.Project{
		ID:          store.NewID(),
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		CreatedBy:   &actx.User.ID,
	}
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		created, err := q.CreateProject(r.Context(), project)
		if err != nil {
			return err
		}
		project = created
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "project.create",
			TargetType: "project", TargetID: created.ID, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project": project})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.store.Q().GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

type projectUpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	var req projectUpdateRequest
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	if req.Name == nil && req.Description == nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "no supported fields supplied")
		return
	}
	update := store.ProjectUpdate{}
	if req.Name != nil {
		name, ok := validateName(*req.Name, 120)
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "name must be 1-120 characters")
			return
		}
		update.Name = &name
	}
	if req.Description != nil {
		if len(*req.Description) > 4000 {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "description is too long")
			return
		}
		desc := strings.TrimSpace(*req.Description)
		update.Description = &desc
	}
	actorID, email, ip := s.actor(r)
	id := r.PathValue("id")
	var updated store.Project
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		var err error
		updated, err = q.UpdateProject(r.Context(), id, update)
		if err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "project.update",
			TargetType: "project", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": updated})
}

func (s *Server) handleArchiveProject(w http.ResponseWriter, r *http.Request) {
	actorID, email, ip := s.actor(r)
	id := r.PathValue("id")
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.ArchiveProject(r.Context(), id); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "project.archive",
			TargetType: "project", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
