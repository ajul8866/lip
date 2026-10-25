package store

import (
	"context"
	"encoding/json"
)

const assetColumns = `id, project_id, kind, value, source, attributes, first_seen, last_seen, created_by`

func scanAsset(row interface{ Scan(...any) error }) (Asset, error) {
	var a Asset
	var attrs []byte
	err := row.Scan(&a.ID, &a.ProjectID, &a.Kind, &a.Value, &a.Source, &attrs,
		&a.FirstSeen, &a.LastSeen, &a.CreatedBy)
	if err != nil {
		return a, mapErr(err)
	}
	a.Attributes = json.RawMessage(attrs)
	return a, nil
}

// UpsertAsset records an asset and refreshes last_seen on repeats. The bool is
// true when a new row was inserted.
func (q *Queries) UpsertAsset(ctx context.Context, a Asset) (Asset, bool, error) {
	attrs, err := jsonArg(a.Attributes)
	if err != nil {
		return Asset{}, false, err
	}
	row := q.db.QueryRow(ctx, `
		INSERT INTO assets (id, project_id, kind, value, source, attributes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project_id, kind, value) DO UPDATE SET
			last_seen = now(),
			source = EXCLUDED.source,
			attributes = assets.attributes || EXCLUDED.attributes
		RETURNING `+assetColumns+`, (xmax = 0) AS inserted`,
		a.ID, a.ProjectID, a.Kind, a.Value, a.Source, attrs, nullablePtr(a.CreatedBy))
	var out Asset
	var attrsOut []byte
	var inserted bool
	err = row.Scan(&out.ID, &out.ProjectID, &out.Kind, &out.Value, &out.Source, &attrsOut,
		&out.FirstSeen, &out.LastSeen, &out.CreatedBy, &inserted)
	if err != nil {
		return Asset{}, false, mapErr(err)
	}
	out.Attributes = json.RawMessage(attrsOut)
	return out, inserted, nil
}

func (q *Queries) GetAsset(ctx context.Context, id string) (Asset, error) {
	row := q.db.QueryRow(ctx, `SELECT `+assetColumns+` FROM assets WHERE id = $1`, id)
	return scanAsset(row)
}

func (q *Queries) ListAssets(ctx context.Context, projectID, kind string, limit, offset int) ([]Asset, int64, error) {
	var total int64
	if err := q.db.QueryRow(ctx, `
		SELECT count(*) FROM assets
		WHERE project_id = $1 AND ($2 = '' OR kind = $2)`, projectID, kind).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.db.Query(ctx, `
		SELECT `+assetColumns+` FROM assets
		WHERE project_id = $1 AND ($2 = '' OR kind = $2)
		ORDER BY kind, value
		LIMIT $3 OFFSET $4`, projectID, kind, limit, offset)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, mapErr(rows.Err())
}

func (q *Queries) IsDeclaredAsset(ctx context.Context, projectID, kind, value string) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM assets
		WHERE project_id = $1 AND kind = $2 AND value = $3)`, projectID, kind, value).Scan(&exists)
	return exists, mapErr(err)
}

func (q *Queries) ListAssetsForReport(ctx context.Context, projectID string) ([]Asset, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+assetColumns+` FROM assets WHERE project_id = $1
		ORDER BY kind, value LIMIT 5000`, projectID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, mapErr(rows.Err())
}

func (q *Queries) DeleteAsset(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM assets WHERE id = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) CountAssets(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM assets`).Scan(&n)
	return n, mapErr(err)
}

func (q *Queries) CountAssetsForProject(ctx context.Context, projectID string) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM assets WHERE project_id = $1`, projectID).Scan(&n)
	return n, mapErr(err)
}
