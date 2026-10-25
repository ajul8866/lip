package store

import (
	"context"
	"encoding/json"
	"time"
)

const auditColumns = `id, actor_id, actor_email, action, target_type, target_id,
	result, ip, metadata, created_at`

func scanAudit(row interface{ Scan(...any) error }) (AuditEntry, error) {
	var a AuditEntry
	var metadata []byte
	err := row.Scan(&a.ID, &a.ActorID, &a.ActorEmail, &a.Action, &a.TargetType, &a.TargetID,
		&a.Result, &a.IP, &metadata, &a.CreatedAt)
	if err != nil {
		return a, mapErr(err)
	}
	a.Metadata = json.RawMessage(metadata)
	return a, nil
}

func (q *Queries) InsertAudit(ctx context.Context, a AuditEntry) (AuditEntry, error) {
	metadata, err := jsonArg(a.Metadata)
	if err != nil {
		return AuditEntry{}, err
	}
	if a.ActorEmail == "" {
		a.ActorEmail = "system"
	}
	row := q.db.QueryRow(ctx, `
		INSERT INTO audit_log (actor_id, actor_email, action, target_type, target_id, result, ip, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+auditColumns,
		nullablePtr(a.ActorID), a.ActorEmail, a.Action, a.TargetType, a.TargetID,
		a.Result, a.IP, metadata)
	return scanAudit(row)
}

type AuditFilter struct {
	ActorID string
	Action  string
	Result  string
	Since   *time.Time
	Until   *time.Time
	Limit   int
	Offset  int
}

func (q *Queries) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, int64, error) {
	var total int64
	if err := q.db.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		WHERE ($1 = '' OR actor_id = $1)
		  AND ($2 = '' OR action = $2)
		  AND ($3 = '' OR result = $3)
		  AND ($4::timestamptz IS NULL OR created_at >= $4)
		  AND ($5::timestamptz IS NULL OR created_at < $5)`,
		f.ActorID, f.Action, f.Result, f.Since, f.Until).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.db.Query(ctx, `
		SELECT `+auditColumns+` FROM audit_log
		WHERE ($1 = '' OR actor_id = $1)
		  AND ($2 = '' OR action = $2)
		  AND ($3 = '' OR result = $3)
		  AND ($4::timestamptz IS NULL OR created_at >= $4)
		  AND ($5::timestamptz IS NULL OR created_at < $5)
		ORDER BY created_at DESC, id DESC
		LIMIT $6 OFFSET $7`,
		f.ActorID, f.Action, f.Result, f.Since, f.Until, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		a, err := scanAudit(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, mapErr(rows.Err())
}

func (q *Queries) PurgeAuditBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := q.db.Exec(ctx, `DELETE FROM audit_log WHERE created_at < $1`, before)
	return tag.RowsAffected(), mapErr(err)
}
