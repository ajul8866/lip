// Package audit records every state-changing action in the database and mirrors
// it to the structured process log. The database write is part of the caller's
// transaction so a mutation cannot commit without its audit row.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/lip-project/lip/internal/store"
)

type Entry struct {
	ActorID    *string
	ActorEmail string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	IP         string
	Metadata   any
}

type Recorder struct {
	logger *slog.Logger
}

func NewRecorder(logger *slog.Logger) *Recorder {
	return &Recorder{logger: logger}
}

// Record persists the audit entry through q and never includes secrets. Call
// it inside the same transaction as the action being recorded.
func (r *Recorder) Record(ctx context.Context, q *store.Queries, e Entry) error {
	if e.Action == "" {
		return fmt.Errorf("audit action is required")
	}
	if e.Result != "success" && e.Result != "failure" && e.Result != "denied" {
		return fmt.Errorf("invalid audit result %q", e.Result)
	}
	if e.ActorEmail == "" {
		e.ActorEmail = "system"
	}
	raw, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	if string(raw) == "null" {
		raw = []byte("{}")
	}
	_, err = q.InsertAudit(ctx, store.AuditEntry{
		ActorID:    e.ActorID,
		ActorEmail: e.ActorEmail,
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		Result:     e.Result,
		IP:         e.IP,
		Metadata:   json.RawMessage(raw),
	})
	if err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	if r.logger != nil {
		attrs := []any{
			"audit", true,
			"action", e.Action,
			"target_type", e.TargetType,
			"target_id", e.TargetID,
			"result", e.Result,
			"actor_email", e.ActorEmail,
			"ip", e.IP,
		}
		switch e.Result {
		case "failure":
			r.logger.Warn("audit", attrs...)
		case "denied":
			r.logger.Warn("audit", attrs...)
		default:
			r.logger.Info("audit", attrs...)
		}
	}
	return nil
}
