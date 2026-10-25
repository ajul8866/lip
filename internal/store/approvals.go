package store

import (
	"context"
)

const approvalColumns = `id, project_id, task_id, action, reason, status,
	requested_by, decided_by, expires_at, decided_at, created_at`

func scanApproval(row interface{ Scan(...any) error }) (Approval, error) {
	var a Approval
	err := row.Scan(&a.ID, &a.ProjectID, &a.TaskID, &a.Action, &a.Reason, &a.Status,
		&a.RequestedBy, &a.DecidedBy, &a.ExpiresAt, &a.DecidedAt, &a.CreatedAt)
	return a, mapErr(err)
}

func (q *Queries) CreateApproval(ctx context.Context, a Approval) (Approval, error) {
	row := q.db.QueryRow(ctx, `
		INSERT INTO approvals (id, project_id, task_id, action, reason, status,
			requested_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7)
		RETURNING `+approvalColumns,
		a.ID, a.ProjectID, a.TaskID, a.Action, a.Reason, nullablePtr(a.RequestedBy), a.ExpiresAt)
	return scanApproval(row)
}

func (q *Queries) GetApproval(ctx context.Context, id string) (Approval, error) {
	row := q.db.QueryRow(ctx, `SELECT `+approvalColumns+` FROM approvals WHERE id = $1`, id)
	return scanApproval(row)
}

func (q *Queries) GetApprovalForUpdate(ctx context.Context, id string) (Approval, error) {
	row := q.db.QueryRow(ctx, `SELECT `+approvalColumns+` FROM approvals WHERE id = $1 FOR UPDATE`, id)
	return scanApproval(row)
}

func (q *Queries) GetPendingApprovalForTask(ctx context.Context, taskID string) (Approval, error) {
	row := q.db.QueryRow(ctx, `
		SELECT `+approvalColumns+` FROM approvals
		WHERE task_id = $1 AND status = 'pending'
		ORDER BY created_at DESC LIMIT 1`, taskID)
	return scanApproval(row)
}

func (q *Queries) ListApprovals(ctx context.Context, projectID, status string, limit, offset int) ([]Approval, int64, error) {
	var total int64
	if err := q.db.QueryRow(ctx, `
		SELECT count(*) FROM approvals
		WHERE project_id = $1 AND ($2 = '' OR status = $2)`, projectID, status).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.db.Query(ctx, `
		SELECT `+approvalColumns+` FROM approvals
		WHERE project_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`, projectID, status, limit, offset)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Approval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, mapErr(rows.Err())
}

func (q *Queries) DecideApproval(ctx context.Context, id, status, decidedBy, reason string) (Approval, error) {
	row := q.db.QueryRow(ctx, `
		UPDATE approvals SET status = $2, decided_by = $3, reason = $4, decided_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING `+approvalColumns, id, status, decidedBy, reason)
	return scanApproval(row)
}

func (q *Queries) ExpireApprovals(ctx context.Context) ([]Approval, error) {
	rows, err := q.db.Query(ctx, `
		UPDATE approvals SET status = 'expired', decided_at = now()
		WHERE status = 'pending' AND expires_at < now()
		RETURNING `+approvalColumns)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []Approval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, mapErr(rows.Err())
}

func (q *Queries) CountPendingApprovals(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM approvals WHERE status = 'pending'`).Scan(&n)
	return n, mapErr(err)
}
