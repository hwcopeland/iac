package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is a connection pool for PostgreSQL backed by pgxpool.
// Using a type alias keeps *db.Pool identical to *pgxpool.Pool so callers
// that need pgxpool features can use it directly without conversion.
type Pool = pgxpool.Pool

// New creates a new connection pool from a PostgreSQL connection URL,
// verifies connectivity via Ping, and returns the pool.
func New(ctx context.Context, url string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("db parse config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db open pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}

	return pool, nil
}
