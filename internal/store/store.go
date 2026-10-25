// Package store is the only package that talks SQL. Every query uses bind
// parameters; dynamic SQL is limited to fixed, allowlisted column fragments.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid")
)

// DBTX is implemented by *pgxpool.Pool and pgx.Tx, allowing the same query
// methods to run inside or outside a transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Q returns a query handle bound to the pool.
func (s *Store) Q() *Queries { return &Queries{db: s.pool} }

// InTx runs fn in a serializable-free read-committed transaction. Mutations and
// their audit rows share the transaction so neither can commit alone.
func (s *Store) InTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := fn(&Queries{db: tx}); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
func (s *Store) Close()                         { s.pool.Close() }

type Queries struct {
	db DBTX
}

// NewQueries exposes a query handle around an existing pool or transaction.
// It is used by tests and background workers that already hold a handle.
func NewQueries(db DBTX) *Queries { return &Queries{db: db} }

// NewID returns an RFC 4122 version 4 UUID string.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}

// AcquireSetupLock serializes first-run admin creation across replicas.
func (q *Queries) AcquireSetupLock(ctx context.Context) error {
	_, err := q.db.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, 721001002)
	return mapErr(err)
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
		case "23503", "23514", "22P02", "22001":
			return fmt.Errorf("%w: %s", ErrInvalid, pgErr.Message)
		}
	}
	return err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullablePtr(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func jsonArg(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}
	if raw, ok := v.(json.RawMessage); ok {
		if len(raw) == 0 {
			return []byte("{}"), nil
		}
		if !json.Valid(raw) {
			return nil, fmt.Errorf("%w: invalid JSON", ErrInvalid)
		}
		return raw, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return b, nil
}
