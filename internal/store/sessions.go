package store

import (
	"context"
)

const sessionColumns = `id, user_id, token_hash, ip, user_agent, created_at, last_seen_at, expires_at, revoked_at`

func scanSession(row interface{ Scan(...any) error }) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.IP, &s.UserAgent,
		&s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.RevokedAt)
	return s, mapErr(err)
}

func (q *Queries) CreateSession(ctx context.Context, s Session) (Session, error) {
	row := q.db.QueryRow(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, ip, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+sessionColumns,
		s.ID, s.UserID, s.TokenHash, s.IP, s.UserAgent, s.ExpiresAt)
	return scanSession(row)
}

// GetSessionWithUser resolves an active session and its user in one query.
func (q *Queries) GetSessionWithUser(ctx context.Context, tokenHash string) (Session, User, error) {
	row := q.db.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.token_hash, s.ip, s.user_agent,
		       s.created_at, s.last_seen_at, s.expires_at, s.revoked_at,
		       u.id, u.email, u.display_name, u.password_hash, u.role, u.locale,
		       u.must_change_password, u.disabled_at, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()
		LIMIT 1`, tokenHash)
	var s Session
	var u User
	err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.IP, &s.UserAgent,
		&s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.RevokedAt,
		&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Role, &u.Locale,
		&u.MustChangePassword, &u.DisabledAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return Session{}, User{}, mapErr(err)
	}
	return s, u, nil
}

func (q *Queries) TouchSession(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE sessions SET last_seen_at = now()
		WHERE id = $1 AND last_seen_at < now() - interval '5 minutes'`, id)
	return mapErr(err)
}

func (q *Queries) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+sessionColumns+` FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, mapErr(rows.Err())
}

func (q *Queries) GetSession(ctx context.Context, id string) (Session, error) {
	row := q.db.QueryRow(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id = $1`, id)
	return scanSession(row)
}

func (q *Queries) RevokeSession(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) RevokeOtherSessions(ctx context.Context, userID, keepID string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND id <> $2 AND revoked_at IS NULL`, userID, keepID)
	return mapErr(err)
}

func (q *Queries) RevokeAllSessions(ctx context.Context, userID string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return mapErr(err)
}

func (q *Queries) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		DELETE FROM sessions
		WHERE expires_at < now() - interval '7 days'
		   OR (revoked_at IS NOT NULL AND revoked_at < now() - interval '7 days')`)
	return tag.RowsAffected(), mapErr(err)
}
