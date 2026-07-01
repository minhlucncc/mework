package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"mework/libs/server/platform/store/sqlite"
)

// Store encapsulates the database handles for either driver backend.
//
// The legacy callers (mework-server main.go, etc.) read `.Pool` for the
// Postgres-backed deployment. For SQLite deployments the new wiring
// surfaces handles via `.SQLite`; `.Pool` is nil. This shape keeps the
// signature of NewStore stable for every caller while letting the
// existing dispatch logic choose the right driver.
type Store struct {
	// Pool is non-nil when the Postgres driver was selected.
	Pool *pgxpool.Pool
	// SQLite is non-nil when the modernc.org/sqlite driver was selected.
	// Callers targeting the offline stack should switch on its presence.
	SQLite *sqlite.Store
}

// NewStore dispatches on the scheme of the DSN to the matching driver.
//
// Supported schemes:
//
//	postgres://…  / postgresql://… → Postgres via pgxpool
//	sqlite://…    / :memory:       / file:… → SQLite via modernc.org/sqlite
//
// Anything else (including the empty string) is a hard error so a
// production deployment with a typo'd DATABASE_URL fails closed instead
// of silently falling back to a default.
func NewStore(ctx context.Context, dsn string) (*Store, error) {
	scheme, err := detectScheme(dsn)
	if err != nil {
		return nil, err
	}

	switch scheme {
	case schemePostgres, schemePostgreSQL:
		return newPostgresStore(ctx, dsn)
	case schemeSQLite, schemeMemory, schemeFile:
		return newSQLiteStore(ctx, dsn)
	default:
		return nil, fmt.Errorf("unsupported DATABASE_URL scheme %q", scheme)
	}
}

// scheme constants used by NewStore dispatch.
const (
	schemePostgres   = "postgres"
	schemePostgreSQL = "postgresql"
	schemeSQLite     = "sqlite"
	schemeMemory     = ":memory:"
	schemeFile       = "file"
)

// detectScheme returns the recognized scheme name for the given DSN or
// an error for empty strings. It does not validate the full URL — that
// is the receiving driver's responsibility.
func detectScheme(dsn string) (string, error) {
	if dsn == "" {
		return "", errors.New("empty DATABASE_URL")
	}

	if dsn == ":memory:" {
		return schemeMemory, nil
	}

	colon := strings.IndexByte(dsn, ':')
	if colon <= 0 {
		// No scheme (e.g. just "data.db"); treat as unsupported.
		return "", fmt.Errorf("unsupported DATABASE_URL %q", dsn)
	}
	scheme := strings.ToLower(dsn[:colon])

	// Strip "//" from sqlite://, postgres://, etc. before parsing body.
	switch scheme {
	case schemePostgres, schemePostgreSQL, schemeSQLite, schemeFile:
		return scheme, nil
	default:
		return scheme, nil
	}
}

// newPostgresStore opens the Postgres pool the legacy way. Kept private
// so the dispatch behaviour is auditable in one place.
func newPostgresStore(ctx context.Context, dsn string) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	// Sane defaults for connection management.
	config.MaxConns = 25
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Store{Pool: pool}, nil
}

// newSQLiteStore opens the SQLite driver via the modernc.org/sqlite
// pure-Go implementation and returns a Store whose SQLite field is
// populated. The Pool field is intentionally nil so callers do not
// accidentally reach for the Postgres path.
func newSQLiteStore(ctx context.Context, dsn string) (*Store, error) {
	s, err := sqlite.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return &Store{SQLite: s}, nil
}

// Close gracefully closes the active driver's pool, if any. Safe to
// call on a partially-initialised Store.
func (s *Store) Close() {
	if s == nil {
		return
	}
	if s.Pool != nil {
		s.Pool.Close()
		s.Pool = nil
	}
	if s.SQLite != nil {
		s.SQLite.Close()
		s.SQLite = nil
	}
}

// OpenSQLDB returns a standard *sql.DB for compatibility (e.g. for
// migrations). For Postgres it delegates to the pgx stdlib driver; for
// SQLite this returns an error so the migration runner can dispatch on
// scheme (see migrate.go).
func OpenSQLDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sql database: %w", err)
	}

	// Verify connectivity.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sql database: %w", err)
	}

	return db, nil
}
