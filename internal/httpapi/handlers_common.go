package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lip-project/lip/internal/audit"
	"github.com/lip-project/lip/internal/store"
)

func (s *Server) actor(r *http.Request) (*string, string, string) {
	actx := authFrom(r)
	id := actx.User.ID
	return &id, actx.User.Email, s.clientIP(r)
}

func (s *Server) recordAudit(ctx context.Context, e audit.Entry) {
	if err := s.store.InTx(ctx, func(q *store.Queries) error {
		return s.audit.Record(ctx, q, e)
	}); err != nil {
		s.log.Error("audit write failed", "action", e.Action, "error", err)
	}
}

func (s *Server) recordSystemAudit(ctx context.Context, action, targetType, targetID, result string, metadata any) {
	s.recordAudit(ctx, audit.Entry{
		Action: action, TargetType: targetType, TargetID: targetID, Result: result, Metadata: metadata,
	})
}

func validateName(v string, max int) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > max {
		return "", false
	}
	return v, true
}

func parseOptionalTime(v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, err
	}
	utc := t.UTC()
	return &utc, nil
}

func isUniqueViolation(err error) bool {
	return errors.Is(err, store.ErrConflict)
}
