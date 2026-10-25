package store

import (
	"context"
	"fmt"
)

const userColumns = `id, email, display_name, password_hash, role, locale,
	must_change_password, disabled_at, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Role,
		&u.Locale, &u.MustChangePassword, &u.DisabledAt, &u.CreatedAt, &u.UpdatedAt)
	return u, mapErr(err)
}

func (q *Queries) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, mapErr(err)
}

func (q *Queries) CreateUser(ctx context.Context, u User) (User, error) {
	row := q.db.QueryRow(ctx, `
		INSERT INTO users (id, email, display_name, password_hash, role, locale, must_change_password)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+userColumns,
		u.ID, u.Email, u.DisplayName, u.PasswordHash, u.Role, u.Locale, u.MustChangePassword)
	return scanUser(row)
}

func (q *Queries) GetUser(ctx context.Context, id string) (User, error) {
	row := q.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (q *Queries) GetUserByEmail(ctx context.Context, email string) (User, error) {
	row := q.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email)
	return scanUser(row)
}

func (q *Queries) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := q.db.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, mapErr(err)
	}

	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, mapErr(rows.Err())
}

func (q *Queries) GetUserForUpdate(ctx context.Context, id string) (User, error) {
	row := q.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1 FOR UPDATE`, id)
	return scanUser(row)
}

type UserUpdate struct {
	DisplayName *string
	Role        *string
	Locale      *string
	Disabled    *bool
}

func (q *Queries) UpdateUser(ctx context.Context, id string, in UserUpdate) (User, error) {
	row := q.db.QueryRow(ctx, `
		UPDATE users SET
			display_name = COALESCE($2, display_name),
			role         = COALESCE($3, role),
			locale       = COALESCE($4, locale),
			disabled_at  = CASE WHEN $5::boolean IS NULL THEN disabled_at
			                    WHEN $5 THEN now() ELSE NULL END,
			updated_at   = now()
		WHERE id = $1
		RETURNING `+userColumns,
		id, in.DisplayName, in.Role, in.Locale, in.Disabled)
	return scanUser(row)
}

func (q *Queries) UpdateUserPassword(ctx context.Context, id, hash string, mustChange bool) error {
	tag, err := q.db.Exec(ctx, `
		UPDATE users SET password_hash = $2, must_change_password = $3, updated_at = now()
		WHERE id = $1`, id, hash, mustChange)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND disabled_at IS NULL`).Scan(&n)
	return n, mapErr(err)
}

func (q *Queries) EnsureEmailFree(ctx context.Context, email, exceptID string) error {
	var exists bool
	err := q.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1) AND id <> $2)`,
		email, exceptID).Scan(&exists)
	if err != nil {
		return mapErr(err)
	}
	if exists {
		return fmt.Errorf("%w: email already registered", ErrConflict)
	}
	return nil
}
