package store

import (
	"context"
)

const projectColumns = `id, name, description, created_by, archived_at, created_at, updated_at`

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.CreatedBy, &p.ArchivedAt, &p.CreatedAt, &p.UpdatedAt)
	return p, mapErr(err)
}

func (q *Queries) CreateProject(ctx context.Context, p Project) (Project, error) {
	row := q.db.QueryRow(ctx, `
		INSERT INTO projects (id, name, description, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING `+projectColumns,
		p.ID, p.Name, p.Description, nullablePtr(p.CreatedBy))
	return scanProject(row)
}

func (q *Queries) GetProject(ctx context.Context, id string) (Project, error) {
	row := q.db.QueryRow(ctx, `SELECT `+projectColumns+` FROM projects WHERE id = $1`, id)
	return scanProject(row)
}

func (q *Queries) ListProjects(ctx context.Context, limit, offset int) ([]Project, int64, error) {
	var total int64
	if err := q.db.QueryRow(ctx, `SELECT count(*) FROM projects WHERE archived_at IS NULL`).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.db.Query(ctx, `
		SELECT `+projectColumns+` FROM projects
		WHERE archived_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, mapErr(rows.Err())
}

type ProjectUpdate struct {
	Name        *string
	Description *string
}

func (q *Queries) UpdateProject(ctx context.Context, id string, in ProjectUpdate) (Project, error) {
	row := q.db.QueryRow(ctx, `
		UPDATE projects SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			updated_at = now()
		WHERE id = $1
		RETURNING `+projectColumns, id, in.Name, in.Description)
	return scanProject(row)
}

func (q *Queries) ArchiveProject(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, `
		UPDATE projects SET archived_at = now(), updated_at = now()
		WHERE id = $1 AND archived_at IS NULL`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) CountProjects(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM projects WHERE archived_at IS NULL`).Scan(&n)
	return n, mapErr(err)
}
