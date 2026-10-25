package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/lip-project/lip/internal/auth"
	"github.com/lip-project/lip/internal/store"
)

type contextKey string

const (
	ctxAuth      contextKey = "auth"
	ctxRequestID contextKey = "request_id"
)

type authContext struct {
	User      store.User
	Session   store.Session
	ViaCookie bool
}

func authFrom(r *http.Request) authContext {
	v, _ := r.Context().Value(ctxAuth).(authContext)
	return v
}

func requestIDFrom(r *http.Request) string {
	v, _ := r.Context().Value(ctxRequestID).(string)
	return v
}

func (s *Server) public(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.Handle(pattern, h)
}

// protected wraps a route with session authentication, cookie CSRF defence,
// and an optional role allowlist.
func (s *Server) protected(mux *http.ServeMux, pattern string, h http.HandlerFunc, roles ...string) {
	mux.Handle(pattern, s.authenticate(s.requirePasswordChanged(s.sameOrigin(s.requireRole(h, roles...)))))
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, viaCookie := s.requestToken(r)
		if token == "" || len(token) > 512 {
			writeErr(w, http.StatusUnauthorized, codeUnauthorized, "authentication required")
			return
		}
		session, user, err := s.store.Q().GetSessionWithUser(r.Context(), auth.HashToken(token))
		if err != nil || user.Disabled() {
			writeErr(w, http.StatusUnauthorized, codeUnauthorized, "invalid or expired session")
			return
		}
		_ = s.store.Q().TouchSession(r.Context(), session.ID)
		ctx := context.WithValue(r.Context(), ctxAuth, authContext{
			User: user, Session: session, ViaCookie: viaCookie,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actx := authFrom(r)
		if !actx.User.MustChangePassword || r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/api/v1/auth/change-password", "/api/v1/auth/logout":
			next.ServeHTTP(w, r)
		default:
			writeErr(w, http.StatusForbidden, codePasswordChange, "change the temporary password before making other changes")
		}
	})
}

func (s *Server) requestToken(r *http.Request) (token string, viaCookie bool) {
	if raw := r.Header.Get("Authorization"); raw != "" {
		if len(raw) < 7 || !strings.EqualFold(raw[:7], "bearer ") {
			return "", false
		}
		return strings.TrimSpace(raw[7:]), false
	}
	cookie, err := r.Cookie(s.cfg.SessionCookie)
	if err != nil {
		return "", true
	}
	return cookie.Value, true
}

func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		actx := authFrom(r)
		if !actx.ViaCookie {
			next.ServeHTTP(w, r)
			return
		}
		if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
			writeErr(w, http.StatusForbidden, codeForbidden, "cross-site request rejected")
			return
		}
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeErr(w, http.StatusForbidden, codeForbidden, "invalid origin")
			return
		}
		allowed := map[string]bool{strings.ToLower(r.Host): true}
		if s.cfg.TrustProxy {
			if fh := firstHeaderValue(r.Header.Get("X-Forwarded-Host")); fh != "" {
				allowed[strings.ToLower(fh)] = true
			}
		}
		if !allowed[strings.ToLower(u.Host)] {
			writeErr(w, http.StatusForbidden, codeForbidden, "cross-origin request rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireRole(next http.Handler, roles ...string) http.Handler {
	if len(roles) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actx := authFrom(r)
		if actx.User.Role == "admin" {
			next.ServeHTTP(w, r)
			return
		}
		for _, role := range roles {
			if role == actx.User.Role {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeErr(w, http.StatusForbidden, codeForbidden, "insufficient role")
	})
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b [8]byte
		_, _ = rand.Read(b[:])
		id := hex.EncodeToString(b[:])
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered", "panic", rec, "path", r.URL.Path,
					"request_id", requestIDFrom(r), "stack", string(debug.Stack()))
				writeErr(w, http.StatusInternalServerError, codeInternal, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}
		s.log.Log(r.Context(), level, "http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", rec.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", s.clientIP(r),
			"request_id", requestIDFrom(r),
		)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; "+
				"base-uri 'self'; form-action 'self'; object-src 'none'")
		if r.TLS != nil || (s.cfg.TrustProxy && strings.EqualFold(firstHeaderValue(r.Header.Get("X-Forwarded-Proto")), "https")) {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func firstHeaderValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Server) setLocaleCookie(w http.ResponseWriter, locale string) {
	if locale != "en" && locale != "ko" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "lip_locale",
		Value:    locale,
		Path:     "/",
		HttpOnly: false,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   365 * 24 * 60 * 60,
	})
}
