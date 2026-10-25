package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/lip-project/lip/internal/store"
)

const (
	codeBadRequest     = "bad_request"
	codeUnauthorized   = "unauthorized"
	codeForbidden      = "forbidden"
	codeNotFound       = "not_found"
	codeConflict       = "conflict"
	codeValidation     = "validation_failed"
	codeRateLimited    = "rate_limited"
	codeSafeMode       = "safe_mode_denied"
	codeEgress         = "egress_denied"
	codeNoProvider     = "no_provider"
	codeProvider       = "provider_error"
	codeInternal       = "internal"
	codeSetupClosed    = "setup_closed"
	codeWeakPassword   = "weak_password"
	codeInvalidLogin   = "invalid_credentials"
	codeLastAdmin      = "last_admin"
	codePasswordChange = "password_change_required"
	codePayloadTooBig  = "payload_too_large"
	codeUnsupported    = "unsupported_media_type"
	codeNotImplemented = "not_implemented"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type listResponse struct {
	Items  any   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, status int, code, message string) {
	writeErrDetails(w, status, code, message, nil)
}

func writeErrDetails(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, Details: details}})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, codeNotFound, "resource not found")
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, codeConflict, "request conflicts with current state")
	case errors.Is(err, store.ErrInvalid):
		writeErr(w, http.StatusUnprocessableEntity, codeValidation, err.Error())
	default:
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeErr(w, http.StatusInternalServerError, codeInternal, "internal server error")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	ct := r.Header.Get("Content-Type")
	if ct != "" {
		mediaType := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
		if mediaType != "application/json" {
			writeErr(w, http.StatusUnsupportedMediaType, codeUnsupported, "Content-Type must be application/json")
			return false
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErr(w, http.StatusRequestEntityTooLarge, codePayloadTooBig, "request body is too large")
			return false
		}
		if errors.Is(err, io.EOF) {
			writeErr(w, http.StatusBadRequest, codeBadRequest, "request body is required")
			return false
		}
		writeErr(w, http.StatusBadRequest, codeBadRequest, "invalid JSON request body")
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeErr(w, http.StatusBadRequest, codeBadRequest, "request body must contain one JSON object")
		return false
	}
	return true
}

func parsePage(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseInt(v, 1, 200); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := parseInt(v, 0, 1_000_000); err == nil {
			offset = n
		}
	}
	return limit, offset
}

func parseInt(v string, min, max int) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	if n < min || n > max {
		return 0, errors.New("out of range")
	}
	return n, nil
}

func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
