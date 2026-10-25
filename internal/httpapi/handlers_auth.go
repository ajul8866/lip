package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/auth"
	"github.com/lip-project/lip/internal/store"
)

type setupRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	ProjectName string `json:"projectName"`
	Locale      string `json:"locale"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if ok, retry := s.setupLimiter.Allow("setup:" + ip); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, codeRateLimited, "too many setup attempts")
		return
	}
	if s.cfg.SetupToken != "" {
		provided := r.Header.Get("X-LIP-Setup-Token")
		if !auth.ConstantTimeEqual(provided, s.cfg.SetupToken) {
			s.recordSystemAudit(r.Context(), "auth.setup", "user", "", "denied", map[string]any{"reason": "bad setup token"})
			writeErr(w, http.StatusForbidden, codeForbidden, "invalid setup token")
			return
		}
	}

	var req setupRequest
	if !decodeJSON(w, r, &req, 16<<10) {
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "invalid email address")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeWeakPassword, err.Error())
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
	projectName := strings.TrimSpace(req.ProjectName)
	if projectName == "" {
		projectName = "Default Project"
	}
	if len(projectName) > 120 {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "projectName must be at most 120 characters")
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

	if n, err := s.store.Q().CountUsers(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, codeInternal, "database unavailable")
		return
	} else if n > 0 {
		writeErr(w, http.StatusConflict, codeSetupClosed, "setup has already been completed")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeWeakPassword, err.Error())
		return
	}
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, codeInternal, "could not create session")
		return
	}
	now := time.Now().UTC()
	expires := now.Add(s.cfg.SessionTTL)
	user := store.User{
		ID:           store.NewID(),
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: hash,
		Role:         "admin",
		Locale:       locale,
	}
	project := store.Project{
		ID:          store.NewID(),
		Name:        projectName,
		Description: "Created during first-run setup.",
		CreatedBy:   &user.ID,
	}
	session := store.Session{
		ID:        store.NewID(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		IP:        ip,
		UserAgent: trimTo(r.UserAgent(), 256),
		ExpiresAt: expires,
	}

	var createdUser store.User
	var createdProject store.Project
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.AcquireSetupLock(r.Context()); err != nil {
			return err
		}
		count, err := q.CountUsers(r.Context())
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: setup closed", store.ErrConflict)
		}
		createdUser, err = q.CreateUser(r.Context(), user)
		if err != nil {
			return err
		}
		createdProject, err = q.CreateProject(r.Context(), project)
		if err != nil {
			return err
		}
		if _, err := q.CreateSession(r.Context(), session); err != nil {
			return err
		}
		if err := s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: &createdUser.ID, ActorEmail: createdUser.Email,
			Action: "auth.setup", TargetType: "user", TargetID: createdUser.ID,
			Result: "success", IP: ip, Metadata: map[string]any{"projectId": createdProject.ID},
		}); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: &createdUser.ID, ActorEmail: createdUser.Email,
			Action: "project.create", TargetType: "project", TargetID: createdProject.ID,
			Result: "success", IP: ip,
		})
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, codeSetupClosed, "setup has already been completed")
			return
		}
		s.log.Error("first-run setup failed", "error", err)
		writeErr(w, http.StatusInternalServerError, codeInternal, "setup failed")
		return
	}
	s.setSessionCookie(w, token, expires)
	s.setLocaleCookie(w, createdUser.Locale)
	writeJSON(w, http.StatusCreated, map[string]any{"user": createdUser, "project": createdProject})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req, 8<<10) {
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil || req.Password == "" || len(req.Password) > auth.MaxPasswordBytes {
		writeErr(w, http.StatusUnauthorized, codeInvalidLogin, "invalid email or password")
		return
	}
	ip := s.clientIP(r)
	key := strings.ToLower(email) + "|" + ip
	if ok, retry := s.loginLimiter.Allow(key); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, codeRateLimited, "too many login attempts")
		return
	}
	if ok, retry := s.emailLimiter.Allow(strings.ToLower(email)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, codeRateLimited, "too many login attempts")
		return
	}

	user, err := s.store.Q().GetUserByEmail(r.Context(), email)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, err)
			return
		}
		_ = auth.VerifyPassword(auth.DummyHash, req.Password)
		s.recordAudit(r.Context(), audit.Entry{
			ActorEmail: email, Action: "auth.login", TargetType: "user", Result: "failure",
			IP: ip, Metadata: map[string]any{"reason": "unknown account"},
		})
		writeErr(w, http.StatusUnauthorized, codeInvalidLogin, "invalid email or password")
		return
	}
	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		s.recordAudit(r.Context(), audit.Entry{
			ActorEmail: email, Action: "auth.login", TargetType: "user", TargetID: user.ID,
			Result: "failure", IP: ip, Metadata: map[string]any{"reason": "bad password"},
		})
		writeErr(w, http.StatusUnauthorized, codeInvalidLogin, "invalid email or password")
		return
	}
	if user.Disabled() {
		s.recordAudit(r.Context(), audit.Entry{
			ActorID: &user.ID, ActorEmail: user.Email, Action: "auth.login", TargetType: "user",
			TargetID: user.ID, Result: "denied", IP: ip, Metadata: map[string]any{"reason": "disabled account"},
		})
		writeErr(w, http.StatusUnauthorized, codeInvalidLogin, "invalid email or password")
		return
	}
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, codeInternal, "could not create session")
		return
	}
	expires := time.Now().UTC().Add(s.cfg.SessionTTL)
	session := store.Session{
		ID:        store.NewID(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		IP:        ip,
		UserAgent: trimTo(r.UserAgent(), 256),
		ExpiresAt: expires,
	}
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		if _, err := q.CreateSession(r.Context(), session); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: &user.ID, ActorEmail: user.Email, Action: "auth.login",
			TargetType: "session", TargetID: session.ID, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.loginLimiter.Reset(key)
	s.emailLimiter.Reset(strings.ToLower(email))
	s.setSessionCookie(w, token, expires)
	s.setLocaleCookie(w, user.Locale)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.RevokeSession(r.Context(), actx.Session.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "auth.logout",
			TargetType: "session", TargetID: actx.Session.ID, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	actx := authFrom(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":               actx.User,
		"sessionId":          actx.Session.ID,
		"safeLocalOnly":      s.cfg.SafeLocalOnly,
		"safeModeSource":     s.cfg.SafeModeSource,
		"maxConcurrentTasks": s.cfg.MaxConcurrent,
		"autoRun":            s.cfg.AutoRun,
	})
}

type updateMeRequest struct {
	DisplayName *string `json:"displayName"`
	Locale      *string `json:"locale"`
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	var req updateMeRequest
	if !decodeJSON(w, r, &req, 8<<10) {
		return
	}
	if req.DisplayName == nil && req.Locale == nil {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "no supported fields supplied")
		return
	}
	actx := authFrom(r)
	update := store.UserUpdate{DisplayName: req.DisplayName}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" || len(name) > 100 {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "displayName must be 1-100 characters")
			return
		}
		update.DisplayName = &name
	}
	if req.Locale != nil {
		locale := strings.ToLower(strings.TrimSpace(*req.Locale))
		if !store.ValidLocale(locale) {
			writeErr(w, http.StatusUnprocessableEntity, codeValidation, "unsupported locale")
			return
		}
		update.Locale = &locale
	}
	actorID, email, ip := s.actor(r)
	var updated store.User
	err := s.store.InTx(r.Context(), func(q *store.Queries) error {
		var err error
		updated, err = q.UpdateUser(r.Context(), actx.User.ID, update)
		if err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "user.update_self",
			TargetType: "user", TargetID: actx.User.ID, Result: "success", IP: ip,
			Metadata: map[string]any{"displayName": update.DisplayName != nil, "locale": update.Locale != nil},
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setLocaleCookie(w, updated.Locale)
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if !decodeJSON(w, r, &req, 8<<10) {
		return
	}
	actx := authFrom(r)
	actorID, email, ip := s.actor(r)
	if !auth.VerifyPassword(actx.User.PasswordHash, req.CurrentPassword) {
		s.recordAudit(r.Context(), audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "auth.change_password",
			TargetType: "user", TargetID: actx.User.ID, Result: "denied", IP: ip,
			Metadata: map[string]any{"reason": "current password mismatch"},
		})
		writeErr(w, http.StatusForbidden, codeForbidden, "current password is incorrect")
		return
	}
	if req.NewPassword == req.CurrentPassword {
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, "new password must differ from the current password")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, codeWeakPassword, err.Error())
		return
	}
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.UpdateUserPassword(r.Context(), actx.User.ID, hash, false); err != nil {
			return err
		}
		if err := q.RevokeOtherSessions(r.Context(), actx.User.ID, actx.Session.ID); err != nil {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "auth.change_password",
			TargetType: "user", TargetID: actx.User.ID, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	actx := authFrom(r)
	sessions, err := s.store.Q().ListSessions(r.Context(), actx.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if sessions == nil {
		sessions = []store.Session{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":            sessions,
		"currentSessionId": actx.Session.ID,
	})
}

func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
	actx := authFrom(r)
	id := r.PathValue("id")
	session, err := s.store.Q().GetSession(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if session.UserID != actx.User.ID {
		s.recordAudit(r.Context(), audit.Entry{
			ActorID: &actx.User.ID, ActorEmail: actx.User.Email, Action: "session.revoke",
			TargetType: "session", TargetID: id, Result: "denied", IP: s.clientIP(r),
		})
		writeErr(w, http.StatusNotFound, codeNotFound, "session not found")
		return
	}
	actorID, email, ip := s.actor(r)
	err = s.store.InTx(r.Context(), func(q *store.Queries) error {
		if err := q.RevokeSession(r.Context(), id); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		return s.audit.Record(r.Context(), q, audit.Entry{
			ActorID: actorID, ActorEmail: email, Action: "session.revoke",
			TargetType: "session", TargetID: id, Result: "success", IP: ip,
		})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if id == actx.Session.ID {
		s.clearSessionCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}
