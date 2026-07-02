package store

import (
	"embed"
	"fmt"
	"log"
	"strings"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// RunMigrations runs all pending migrations in the embedded FS against the given DSN.
// For SQLite DSNs (sqlite://, :memory:, file:), the Postgres-specific goose migrations
// are skipped — the SQLite store handles its own schema via ApplyMigrations instead.
func RunMigrations(dsn string) error {
	// For SQLite, skip goose migrations (SQL is Postgres-specific: uuid-ossp,
	// gen_random_uuid, TIMESTAMPTZ, JSONB). The SQLite store applies its own
	// compatible schema during Open.
	if isSQLiteDSN(dsn) {
		log.Println("SQLite DSN detected — skipping Postgres-specific migrations")
		return nil
	}

	db, err := OpenSQLDB(dsn)
	if err != nil {
		return fmt.Errorf("failed to open DB for migrations: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Disable goose's verbose logging to keep tests and daemon logs clean,
	// but keeping errors visible.
	goose.SetLogger(goose.NopLogger())

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations up: %w", err)
	}

	return nil
}

// isSQLiteDSN returns true if the DSN targets a SQLite database.
func isSQLiteDSN(dsn string) bool {
	if dsn == ":memory:" || dsn == "" {
		return true
	}
	low := strings.ToLower(dsn)
	return strings.HasPrefix(low, "sqlite:") || strings.HasPrefix(low, "file:")
}

// RollbackMigrations rolls back all migrations (Down) against the given DSN.
func RollbackMigrations(dsn string) error {
	db, err := OpenSQLDB(dsn)
	if err != nil {
		return fmt.Errorf("failed to open DB for rollback: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	goose.SetLogger(goose.NopLogger())

	if err := goose.DownTo(db, "migrations", 0); err != nil {
		return fmt.Errorf("failed to run migrations down: %w", err)
	}

	return nil
}
