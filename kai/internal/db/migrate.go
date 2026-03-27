package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver for database/sql
)

// Embed the entire migrations directory so that new migration files are picked
// up automatically without editing this file.
//
//go:embed migrations
var migrationFS embed.FS

// Migrate runs all pending SQL migrations embedded in internal/db/migrations/.
// It is idempotent: already-applied migrations are skipped.
func Migrate(ctx context.Context, url string) error {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("db migrate open: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("db migrate ping: %w", err)
	}

	src, err := iofs.New(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("db migrate iofs source: %w", err)
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("db migrate driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("db migrate new: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("db migrate up: %w", err)
	}

	return nil
}
