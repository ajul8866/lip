package store

import (
	"context"
	"encoding/json"
)

const settingColumns = `key, value, updated_by, updated_at`

func scanSetting(row interface{ Scan(...any) error }) (Setting, error) {
	var s Setting
	var value []byte
	err := row.Scan(&s.Key, &value, &s.UpdatedBy, &s.UpdatedAt)
	if err != nil {
		return s, mapErr(err)
	}
	s.Value = json.RawMessage(value)
	return s, nil
}

func (q *Queries) ListSettings(ctx context.Context) ([]Setting, error) {
	rows, err := q.db.Query(ctx, `SELECT `+settingColumns+` FROM settings ORDER BY key`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []Setting
	for rows.Next() {
		s, err := scanSetting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, mapErr(rows.Err())
}

func (q *Queries) GetSetting(ctx context.Context, key string) (Setting, error) {
	row := q.db.QueryRow(ctx, `SELECT `+settingColumns+` FROM settings WHERE key = $1`, key)
	return scanSetting(row)
}

func (q *Queries) UpsertSetting(ctx context.Context, key string, value json.RawMessage, updatedBy *string) (Setting, error) {
	v, err := jsonArg(value)
	if err != nil {
		return Setting{}, err
	}
	row := q.db.QueryRow(ctx, `
		INSERT INTO settings (key, value, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
		RETURNING `+settingColumns, key, v, nullablePtr(updatedBy))
	return scanSetting(row)
}

const providerColumns = `id, name, kind, base_url, model, api_key_env, enabled, is_default,
	config, created_at, updated_at`

func scanProvider(row interface{ Scan(...any) error }) (LLMProvider, error) {
	var p LLMProvider
	var config []byte
	err := row.Scan(&p.ID, &p.Name, &p.Kind, &p.BaseURL, &p.Model, &p.APIKeyEnv,
		&p.Enabled, &p.IsDefault, &config, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, mapErr(err)
	}
	p.Config = json.RawMessage(config)
	return p, nil
}

func (q *Queries) ListProviders(ctx context.Context) ([]LLMProvider, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+providerColumns+` FROM llm_providers
		ORDER BY is_default DESC, created_at ASC`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []LLMProvider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, mapErr(rows.Err())
}

func (q *Queries) GetProvider(ctx context.Context, id string) (LLMProvider, error) {
	row := q.db.QueryRow(ctx, `SELECT `+providerColumns+` FROM llm_providers WHERE id = $1`, id)
	return scanProvider(row)
}

func (q *Queries) GetDefaultProvider(ctx context.Context) (LLMProvider, error) {
	row := q.db.QueryRow(ctx, `
		SELECT `+providerColumns+` FROM llm_providers
		WHERE is_default AND enabled LIMIT 1`)
	return scanProvider(row)
}

func (q *Queries) CreateProvider(ctx context.Context, p LLMProvider) (LLMProvider, error) {
	config, err := jsonArg(p.Config)
	if err != nil {
		return LLMProvider{}, err
	}
	row := q.db.QueryRow(ctx, `
		INSERT INTO llm_providers (id, name, kind, base_url, model, api_key_env, enabled, is_default, config)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+providerColumns,
		p.ID, p.Name, p.Kind, p.BaseURL, p.Model, p.APIKeyEnv, p.Enabled, p.IsDefault, config)
	return scanProvider(row)
}

type ProviderUpdate struct {
	Name      *string
	Kind      *string
	BaseURL   *string
	Model     *string
	APIKeyEnv *string
	Enabled   *bool
	IsDefault *bool
	Config    json.RawMessage
}

func (q *Queries) UpdateProvider(ctx context.Context, id string, in ProviderUpdate) (LLMProvider, error) {
	config, err := jsonArg(in.Config)
	if err != nil {
		return LLMProvider{}, err
	}
	row := q.db.QueryRow(ctx, `
		UPDATE llm_providers SET
			name = COALESCE($2, name),
			kind = COALESCE($3, kind),
			base_url = COALESCE($4, base_url),
			model = COALESCE($5, model),
			api_key_env = COALESCE($6, api_key_env),
			enabled = COALESCE($7, enabled),
			is_default = COALESCE($8, is_default),
			config = CASE WHEN $9::jsonb = '{}'::jsonb THEN config ELSE $9::jsonb END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+providerColumns,
		id, in.Name, in.Kind, in.BaseURL, in.Model, in.APIKeyEnv, in.Enabled, in.IsDefault, config)
	return scanProvider(row)
}

func (q *Queries) ClearDefaultProviders(ctx context.Context, exceptID string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE llm_providers SET is_default = false, updated_at = now()
		WHERE is_default AND id <> $1`, exceptID)
	return mapErr(err)
}

func (q *Queries) DeleteProvider(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM llm_providers WHERE id = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) CountProviders(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM llm_providers`).Scan(&n)
	return n, mapErr(err)
}
