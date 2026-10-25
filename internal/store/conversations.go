package store

import (
	"context"
	"encoding/json"
)

const conversationColumns = `id, project_id, title, created_by, created_at, updated_at`

func scanConversation(row interface{ Scan(...any) error }) (Conversation, error) {
	var c Conversation
	err := row.Scan(&c.ID, &c.ProjectID, &c.Title, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	return c, mapErr(err)
}

func (q *Queries) CreateConversation(ctx context.Context, c Conversation) (Conversation, error) {
	row := q.db.QueryRow(ctx, `
		INSERT INTO conversations (id, project_id, title, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING `+conversationColumns,
		c.ID, c.ProjectID, c.Title, nullablePtr(c.CreatedBy))
	return scanConversation(row)
}

func (q *Queries) GetConversation(ctx context.Context, id string) (Conversation, error) {
	row := q.db.QueryRow(ctx, `SELECT `+conversationColumns+` FROM conversations WHERE id = $1`, id)
	return scanConversation(row)
}

func (q *Queries) ListConversations(ctx context.Context, projectID string, limit, offset int) ([]Conversation, int64, error) {
	var total int64
	if err := q.db.QueryRow(ctx, `SELECT count(*) FROM conversations WHERE project_id = $1`, projectID).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.db.Query(ctx, `
		SELECT `+conversationColumns+` FROM conversations
		WHERE project_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3`, projectID, limit, offset)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, mapErr(rows.Err())
}

func (q *Queries) UpdateConversationTitle(ctx context.Context, id, title string) (Conversation, error) {
	row := q.db.QueryRow(ctx, `
		UPDATE conversations SET title = $2, updated_at = now()
		WHERE id = $1
		RETURNING `+conversationColumns, id, title)
	return scanConversation(row)
}

func (q *Queries) TouchConversation(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, id)
	return mapErr(err)
}

func (q *Queries) DeleteConversation(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const messageColumns = `id, conversation_id, role, content, metadata, created_by, created_at`

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var metadata []byte
	err := row.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &metadata, &m.CreatedBy, &m.CreatedAt)
	if err != nil {
		return m, mapErr(err)
	}
	m.Metadata = json.RawMessage(metadata)
	return m, nil
}

func (q *Queries) CreateMessage(ctx context.Context, m Message) (Message, error) {
	meta, err := jsonArg(m.Metadata)
	if err != nil {
		return Message{}, err
	}
	row := q.db.QueryRow(ctx, `
		INSERT INTO messages (id, conversation_id, role, content, metadata, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+messageColumns,
		m.ID, m.ConversationID, m.Role, m.Content, meta, nullablePtr(m.CreatedBy))
	return scanMessage(row)
}

func (q *Queries) ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]Message, int64, error) {
	var total int64
	if err := q.db.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, conversationID).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.db.Query(ctx, `
		SELECT `+messageColumns+` FROM messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3`, conversationID, limit, offset)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, mapErr(rows.Err())
}

func (q *Queries) CountMessages(ctx context.Context, conversationID string) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, conversationID).Scan(&n)
	return n, mapErr(err)
}
