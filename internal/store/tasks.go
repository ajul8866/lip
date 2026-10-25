package store

import (
	"context"
	"encoding/json"
	"fmt"
)

const taskColumns = `id, project_id, conversation_id, title, description, tool, parameters,
	risk_level, status, attempts, last_error, result, created_by, approved_by,
	started_at, finished_at, created_at, updated_at`

const taskColumnsT = `t.id, t.project_id, t.conversation_id, t.title, t.description, t.tool, t.parameters,
	t.risk_level, t.status, t.attempts, t.last_error, t.result, t.created_by, t.approved_by,
	t.started_at, t.finished_at, t.created_at, t.updated_at`

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	var params, result []byte
	err := row.Scan(&t.ID, &t.ProjectID, &t.ConversationID, &t.Title, &t.Description,
		&t.Tool, &params, &t.RiskLevel, &t.Status, &t.Attempts, &t.LastError, &result,
		&t.CreatedBy, &t.ApprovedBy, &t.StartedAt, &t.FinishedAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, mapErr(err)
	}
	t.Parameters = json.RawMessage(params)
	t.Result = json.RawMessage(result)
	return t, nil
}

func (q *Queries) CreateTask(ctx context.Context, t Task) (Task, error) {
	params, err := jsonArg(t.Parameters)
	if err != nil {
		return Task{}, err
	}
	result, err := jsonArg(t.Result)
	if err != nil {
		return Task{}, err
	}
	row := q.db.QueryRow(ctx, `
		INSERT INTO tasks (id, project_id, conversation_id, title, description, tool,
			parameters, risk_level, status, result, created_by, approved_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+taskColumns,
		t.ID, t.ProjectID, nullablePtr(t.ConversationID), t.Title, t.Description, t.Tool,
		params, t.RiskLevel, t.Status, result, nullablePtr(t.CreatedBy), nullablePtr(t.ApprovedBy))
	return scanTask(row)
}

func (q *Queries) GetTask(ctx context.Context, id string) (Task, error) {
	row := q.db.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = $1`, id)
	return scanTask(row)
}

func (q *Queries) GetTaskForUpdate(ctx context.Context, id string) (Task, error) {
	row := q.db.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = $1 FOR UPDATE`, id)
	return scanTask(row)
}

type TaskFilter struct {
	Status string
	Limit  int
	Offset int
}

func (q *Queries) ListTasks(ctx context.Context, projectID string, f TaskFilter) ([]Task, int64, error) {
	var total int64
	if err := q.db.QueryRow(ctx, `
		SELECT count(*) FROM tasks
		WHERE project_id = $1 AND ($2 = '' OR status = $2)`, projectID, f.Status).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.db.Query(ctx, `
		SELECT `+taskColumns+` FROM tasks
		WHERE project_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`, projectID, f.Status, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, mapErr(rows.Err())
}

func (q *Queries) ListAllTasks(ctx context.Context, limit int) ([]Task, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+taskColumns+` FROM tasks ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, mapErr(rows.Err())
}

type TaskUpdate struct {
	Title       *string
	Description *string
	Parameters  json.RawMessage
}

func (q *Queries) UpdateTask(ctx context.Context, id string, in TaskUpdate) (Task, error) {
	params, err := jsonArg(in.Parameters)
	if err != nil {
		return Task{}, err
	}
	row := q.db.QueryRow(ctx, `
		UPDATE tasks SET
			title = COALESCE($2, title),
			description = COALESCE($3, description),
			parameters = CASE WHEN $4::jsonb = '{}'::jsonb THEN parameters ELSE $4::jsonb END,
			updated_at = now()
		WHERE id = $1 AND status IN ('queued', 'awaiting_approval', 'blocked')
		RETURNING `+taskColumns, id, in.Title, in.Description, params)
	return scanTask(row)
}

// ClaimQueued atomically marks one runnable task as running. The caller must
// already hold a concurrency slot.
func (q *Queries) ClaimQueued(ctx context.Context) (Task, error) {
	row := q.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM tasks
			WHERE status = 'queued'
			  AND (risk_level = 'passive' OR approved_by IS NOT NULL)
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE tasks t SET
			status = 'running',
			attempts = attempts + 1,
			started_at = now(),
			finished_at = NULL,
			last_error = '',
			updated_at = now()
		FROM candidate c
		WHERE t.id = c.id
		RETURNING `+taskColumnsT)
	return scanTask(row)
}

func (q *Queries) FinishTask(ctx context.Context, id, status string, result any, lastErr string) (Task, error) {
	if status != "succeeded" && status != "failed" && status != "cancelled" && status != "blocked" {
		return Task{}, fmt.Errorf("%w: invalid terminal status", ErrInvalid)
	}
	res, err := jsonArg(result)
	if err != nil {
		return Task{}, err
	}
	row := q.db.QueryRow(ctx, `
		UPDATE tasks SET status = $2, result = $3, last_error = $4,
			finished_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'running'
		RETURNING `+taskColumns, id, status, res, lastErr)
	return scanTask(row)
}

func (q *Queries) ApproveTask(ctx context.Context, id, approverID string) (Task, error) {
	row := q.db.QueryRow(ctx, `
		UPDATE tasks SET status = 'queued', approved_by = $2, updated_at = now()
		WHERE id = $1 AND status = 'awaiting_approval'
		RETURNING `+taskColumns, id, approverID)
	return scanTask(row)
}

func (q *Queries) CancelTask(ctx context.Context, id string) (Task, error) {
	row := q.db.QueryRow(ctx, `
		UPDATE tasks SET status = 'cancelled', finished_at = now(), updated_at = now()
		WHERE id = $1 AND status IN ('queued', 'awaiting_approval', 'blocked')
		RETURNING `+taskColumns, id)
	return scanTask(row)
}

func (q *Queries) BlockTask(ctx context.Context, id, reason string) (Task, error) {
	row := q.db.QueryRow(ctx, `
		UPDATE tasks SET status = 'blocked', last_error = $2, updated_at = now()
		WHERE id = $1 AND status IN ('queued', 'awaiting_approval')
		RETURNING `+taskColumns, id, reason)
	return scanTask(row)
}

// RequeueStale recovers tasks whose runner vanished mid-execution.
func (q *Queries) RequeueStale(ctx context.Context, olderThanMinutes int) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE tasks SET status = 'queued', last_error = 'runner heartbeat lost; requeued', updated_at = now()
		WHERE status = 'running' AND started_at < now() - make_interval(mins => $1)`, olderThanMinutes)
	return tag.RowsAffected(), mapErr(err)
}

func (q *Queries) CountTasks(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM tasks`).Scan(&n)
	return n, mapErr(err)
}

func (q *Queries) CountTasksByStatus(ctx context.Context, projectID string) ([]CountByKey, error) {
	rows, err := q.db.Query(ctx, `
		SELECT status, count(*) FROM tasks
		WHERE ($1 = '' OR project_id = $1)
		GROUP BY status ORDER BY status`, projectID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []CountByKey
	for rows.Next() {
		var c CountByKey
		if err := rows.Scan(&c.Key, &c.Count); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, c)
	}
	return out, mapErr(rows.Err())
}

func (q *Queries) DeleteTask(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM tasks WHERE id = $1 AND status <> 'running'`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) CountRunningTasks(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE status = 'running'`).Scan(&n)
	return n, mapErr(err)
}
