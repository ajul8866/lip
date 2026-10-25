package store

import (
	"context"
	"encoding/json"
)

const findingColumns = `id, project_id, task_id, asset_id, title, description, severity,
	status, source, evidence, created_by, created_at, updated_at`

func scanFinding(row interface{ Scan(...any) error }) (Finding, error) {
	var f Finding
	var evidence []byte
	err := row.Scan(&f.ID, &f.ProjectID, &f.TaskID, &f.AssetID, &f.Title, &f.Description,
		&f.Severity, &f.Status, &f.Source, &evidence, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return f, mapErr(err)
	}
	f.Evidence = json.RawMessage(evidence)
	return f, nil
}

func (q *Queries) CreateFinding(ctx context.Context, f Finding) (Finding, error) {
	evidence, err := jsonArg(f.Evidence)
	if err != nil {
		return Finding{}, err
	}
	row := q.db.QueryRow(ctx, `
		INSERT INTO findings (id, project_id, task_id, asset_id, title, description,
			severity, status, source, evidence, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+findingColumns,
		f.ID, f.ProjectID, nullablePtr(f.TaskID), nullablePtr(f.AssetID), f.Title, f.Description,
		f.Severity, f.Status, f.Source, evidence, nullablePtr(f.CreatedBy))
	return scanFinding(row)
}

func (q *Queries) GetFinding(ctx context.Context, id string) (Finding, error) {
	row := q.db.QueryRow(ctx, `SELECT `+findingColumns+` FROM findings WHERE id = $1`, id)
	return scanFinding(row)
}

type FindingFilter struct {
	Severity string
	Status   string
	Limit    int
	Offset   int
}

func (q *Queries) ListFindings(ctx context.Context, projectID string, f FindingFilter) ([]Finding, int64, error) {
	var total int64
	if err := q.db.QueryRow(ctx, `
		SELECT count(*) FROM findings
		WHERE project_id = $1
		  AND ($2 = '' OR severity = $2)
		  AND ($3 = '' OR status = $3)`, projectID, f.Severity, f.Status).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.db.Query(ctx, `
		SELECT `+findingColumns+` FROM findings
		WHERE project_id = $1
		  AND ($2 = '' OR severity = $2)
		  AND ($3 = '' OR status = $3)
		ORDER BY CASE severity
			WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2
			WHEN 'low' THEN 3 ELSE 4 END, created_at DESC
		LIMIT $4 OFFSET $5`, projectID, f.Severity, f.Status, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		finding, err := scanFinding(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, finding)
	}
	return out, total, mapErr(rows.Err())
}

func (q *Queries) ListFindingsForReport(ctx context.Context, projectID string) ([]Finding, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+findingColumns+` FROM findings
		WHERE project_id = $1
		ORDER BY CASE severity
			WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2
			WHEN 'low' THEN 3 ELSE 4 END, created_at ASC`, projectID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, mapErr(rows.Err())
}

func (q *Queries) ListRecentFindings(ctx context.Context, limit int) ([]Finding, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+findingColumns+` FROM findings ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, mapErr(rows.Err())
}

type FindingUpdate struct {
	Title       *string
	Description *string
	Severity    *string
	Status      *string
	Evidence    json.RawMessage
}

func (q *Queries) UpdateFinding(ctx context.Context, id string, in FindingUpdate) (Finding, error) {
	evidence, err := jsonArg(in.Evidence)
	if err != nil {
		return Finding{}, err
	}
	row := q.db.QueryRow(ctx, `
		UPDATE findings SET
			title = COALESCE($2, title),
			description = COALESCE($3, description),
			severity = COALESCE($4, severity),
			status = COALESCE($5, status),
			evidence = CASE WHEN $6::jsonb = '{}'::jsonb THEN evidence ELSE $6::jsonb END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+findingColumns,
		id, in.Title, in.Description, in.Severity, in.Status, evidence)
	return scanFinding(row)
}

func (q *Queries) DeleteFinding(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM findings WHERE id = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) CountFindings(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM findings`).Scan(&n)
	return n, mapErr(err)
}

func (q *Queries) CountOpenFindings(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM findings WHERE status = 'open'`).Scan(&n)
	return n, mapErr(err)
}

func (q *Queries) CountFindingsBySeverity(ctx context.Context, projectID string) ([]CountByKey, error) {
	rows, err := q.db.Query(ctx, `
		SELECT severity, count(*) FROM findings
		WHERE ($1 = '' OR project_id = $1) AND status = 'open'
		GROUP BY severity ORDER BY severity`, projectID)
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
