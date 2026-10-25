package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/auth"
	"github.com/lip-project/lip/internal/store"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.Q().ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if users == nil {
		users = []store.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

type createUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Locale      string `json:"locale"`
	Password    string `json:"password"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid email address")
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = "analyst"
	}
	if !store.ValidRole(role) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid role")
		return
	}
	locale := strings.ToLower(strings.TrimSpace(req.Locale))
	if locale == "" {
		locale = s.cfg.DefaultLocale
	}
	if !store.ValidLocale(locale) {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "unsupported locale")
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = strings.SplitN(email, "@", 2)[0]
	}
	if len(displayName) > 100 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "displayName must be at most 100 characters")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeWeakPassword, err.Error())
		return
	}
	actorID, actorEmail, ip := s.actor(r)
	user := store.User{
		ID:                 store.NewID(),
		Email:              email,
		DisplayName:        displayName,
		PasswordHash:       hash,
		Role:               role,
		Locale:             locale,
		MustChangePassword: true,
	}
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.EnsureEmailFree(r.Context(), email, ""); err != nil {
			return err
		}
		created, err := q.CreateUser(r.Context(), user)
		if err != nil {
			return err
		}
		user = created
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: actorEmail, Action: "user.create",
			TargetType: "user", TargetID: created.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"role": created.Role, "email": created.Email},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName"`
	Role        *string `json:"role"`
	Locale      *string `json:"locale"`
	Disabled    *bool   `json:"disabled"`
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	var req updateUserRequest
	if !decodeJSON(w, r, &req, 8<<10) {
		return
	}
	if req.DisplayName == nil && req.Role == nil && req.Locale == nil && req.Disabled == nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "no supported fields supplied")
		return
	}
	update := store.UserUpdate{}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" || len(name) > 100 {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "displayName must be 1-100 characters")
			return
		}
		update.DisplayName = &name
	}
	if req.Role != nil {
		role := strings.ToLower(strings.TrimSpace(*req.Role))
		if !store.ValidRole(role) {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid role")
			return
		}
		update.Role = &role
	}
	if req.Locale != nil {
		locale := strings.ToLower(strings.TrimSpace(*req.Locale))
		if !store.ValidLocale(locale) {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "unsupported locale")
			return
		}
		update.Locale = &locale
	}
	update.Disabled = req.Disabled

	actorID, actorEmail, ip := s.actor(r)
	targetID := r.PathValue("id")
	var updated store.User
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		target, err := q.GetUserForUpdate(r.Context(), targetID)
		if err != nil {
			return err
		}
		nextRole := target.Role
		if update.Role != nil {
			nextRole = *update.Role
		}
		nextDisabled := target.Disabled()
		if update.Disabled != nil {
			nextDisabled = *update.Disabled
		}
		if target.Role == "admin" && (nextRole != "admin" || nextDisabled) {
			admins, err := q.CountAdmins(r.Context())
			if err != nil {
				return err
			}
			if admins <= 1 {
				return errLastAdmin
			}
		}
		updated, err = q.UpdateUser(r.Context(), targetID, update)
		if err != nil {
			return err
		}
		if updated.Disabled() {
			if err := q.RevokeAllSessions(r.Context(), updated.ID); err != nil {
				return err
			}
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: actorEmail, Action: "user.update",
			TargetType: "user", TargetID: targetID, Result: "success", IP: ip,
			Metadata: map[string]any{"role": updated.Role, "disabled": updated.Disabled()},
		})
	})
	if err != nil {
		if errors.Is(err, errLastAdmin) {
			writeErr(w, http.StatusConflict, codeLastAdmin, "cannot remove the last active administrator")
			return
		}
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

var errLastAdmin = errors.New("last active admin")

type resetPasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if !decodeJSON(w, r, &req, 8<<10) {
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeWeakPassword, err.Error())
		return
	}
	targetID := r.PathValue("id")
	actorID, actorEmail, ip := s.actor(r)
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		if _, err := q.GetUserForUpdate(r.Context(), targetID); err != nil {
			return err
		}
		if err := q.UpdateUserPassword(r.Context(), targetID, hash, true); err != nil {
			return err
		}
		if err := q.RevokeAllSessions(r.Context(), targetID); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: actorEmail, Action: "user.reset_password",
			TargetType: "user", TargetID: targetID, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if targetID == authFrom(r).User.ID {
		s.clearSessionCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}
