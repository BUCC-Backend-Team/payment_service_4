package database

import (
	"context"
	"database/sql"
)

// Store pairs the connection pool with the sqlc-generated Queries.
// It is hand-written and will not be touched by `sqlc generate`.
type Store struct {
	*Queries
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		Queries: New(db),
		DB:      db,
	}
}

func (s *Store) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return s.DB.BeginTx(ctx, opts)
}

func (s *Store) WithTx(tx *sql.Tx) *Queries {
	return New(tx)
}
