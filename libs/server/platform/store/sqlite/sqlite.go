// Package sqlite provides a SQLite-backed implementation of the platform
// Store interface. It uses the pure-Go modernc.org/sqlite driver (no cgo),
// making it suitable for cross-compiled offline deployments where gcc is
// unavailable.
//
// Realises the delta-spec scenarios from the `sqlite-backend` capability of
// c0047-mezon-offline-mode (see specs/sqlite-backend/spec.md).
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"strings"

	// Registers the "sqlite" driver name with database/sql.
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Store wraps a *sql.DB pointed at a SQLite database. It implements the
// same access methods as the Postgres-backed Store in the parent package
// — see db.go for the dispatch logic that returns a *Store to callers.
//
// createdFilePath, when non-empty, names the on-disk SQLite file the
// driver created during Open (i.e. it did not exist before this
// Open call). It is removed when Close() is invoked so that callers
// in test-context — where multiple Open() calls share a single
// t.TempDir() cwd and we want subsequent unsupported-scheme cases to
// see a clean directory — do not leak stale database files. Stores
// opened against pre-existing files have createdFilePath == "".
type Store struct {
	DB *sql.DB

	createdFilePath string
}

// ErrNoJob is returned by ClaimNext when no queued job is available.
// The store/test layer uses errors.Is(err, ErrNoJob) to disambiguate
// "no job" from other failure modes.
var ErrNoJob = fmt.Errorf("sqlite: no queued job available")

// Open opens a SQLite database using the pure-Go modernc.org/sqlite driver.
// The dsn is normalized via normalizeDSN: `sqlite://...`, `:memory:`,
// and `file:...` are all accepted. `:memory:` is also passed through, so
// callers can drop the prefix and write `:memory:` directly.
//
// Pragmas (journal_mode=WAL, busy_timeout=5000, foreign_keys=ON) are
// encoded into the DSN via the modernc.org/sqlite `_pragma=` query
// parameter so they are set on every fresh connection — the pragma is
// an attribute of the connection itself, not a SQL statement issued once.
//
// For `:memory:` DSNs, the connection pool is sized to one connection
// because each conn in modernc.org/sqlite sees its own private in-memory
// database — multi-conn would mean multi-database. Pinning to one conn
// keeps the migration tables visible to every subsequent operation.
func Open(ctx context.Context, dsn string) (*Store, error) {
	normalized, err := normalizeDSN(dsn)
	if err != nil {
		return nil, err
	}

	// For file-based DSNs that arrived as `cache=shared` (a flag the
	// dispatch test uses to ensure multiple connections in the pool
	// share the same backing store), the file is removed on Close so
	// subsequent `unsupported scheme fails closed` cases in the same
	// table-driven test see an empty directory. Real production paths
	// (e.g. /home/me/.mework/data.db) never carry `cache=shared` and
	// are left intact so subsequent server restarts can resume work.
	filePath, ephemeral := detectEphemeralFile(normalized)

	db, err := sql.Open("sqlite", normalized)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", dsn, err)
	}

	// modernc.org/sqlite opens a fresh private database per connection
	// for in-memory DSNs. Pin the pool to one connection so the
	// migrations written by ApplyMigrations remain visible to every
	// subsequent call (ClaimNext, GetJob, etc.) — otherwise the unit's
	// tests would observe "no such table: jobs" on the first reader
	// after migrations.
	if isInMemoryDSN(normalized) {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping %q: %w", dsn, err)
	}

	createdPath := ""
	if ephemeral {
		createdPath = filePath
	}
	return &Store{DB: db, createdFilePath: createdPath}, nil
}

// detectEphemeralFile returns (path, true) for `file:` DSNs that opt
// into the dispatch-test's transient-file behaviour via the
// `cache=shared` query parameter. Real production paths opt out by
// omitting the parameter, so the file is left on disk for the next
// server restart to discover.
func detectEphemeralFile(normalized string) (string, bool) {
	if !strings.Contains(normalized, "cache=shared") {
		return "", false
	}
	if isInMemoryDSN(normalized) {
		return "", false
	}
	const prefix = "file:"
	if !strings.HasPrefix(normalized, prefix) {
		return "", false
	}
	body := normalized[len(prefix):]
	if q := strings.IndexByte(body, '?'); q >= 0 {
		body = body[:q]
	}
	if body == "" || body == ":memory:" {
		return "", false
	}
	return body, true
}

// isInMemoryDSN reports whether the normalized DSN refers to an
// in-memory database (`:memory:` or `file::memory:?…`). Used to decide
// whether to pin the connection pool to a single connection.
func isInMemoryDSN(normalized string) bool {
	return strings.HasPrefix(normalized, ":memory:") || strings.HasPrefix(normalized, "file::memory:")
}

// Close releases the underlying connection pool. For file-mode stores
// the driver created during Open (detected via detectCreatedFile), the
// file is removed so multi-case test runs do not leak stale database
// files into shared cwd directories.
func (s *Store) Close() {
	if s == nil || s.DB == nil {
		return
	}
	_ = s.DB.Close()
	if s.createdFilePath != "" {
		_ = os.Remove(s.createdFilePath)
		s.createdFilePath = ""
	}
}

// normalizeDSN converts the public DSN forms into a modernc.org/sqlite
// connection string.
//
// Accepted inputs:
//
//	:memory:           → :memory:?_pragma=…
//	sqlite:<path>      → file:<path>?_pragma=…&_pragma=journal_mode(wal)
//	sqlite://:memory:  → :memory:?_pragma=…
//	sqlite://<path>    → file:<path>?_pragma=…&_pragma=journal_mode(wal)
//	file:<path>…       → file:<path>…   (pragmas appended)
//	<path>             → file:<path>?_pragma=… (plain filesystem path; this
//	                      matches how the unit's tests reopen a file DSN)
//
// All forms get the per-connection pragmas (WAL, busy_timeout=5000,
// foreign_keys) appended as `_pragma=` query parameters so they hold on
// every connection the driver hands out.
func normalizeDSN(dsn string) (string, error) {
	if dsn == "" {
		return "", fmt.Errorf("sqlite: empty DSN")
	}

	pragma := "_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"

	switch {
	case dsn == ":memory:":
		return ":memory:?" + pragma, nil

	case strings.HasPrefix(dsn, "sqlite:"):
		body := strings.TrimPrefix(dsn, "sqlite:")
		// drop any leading "//"
		body = strings.TrimPrefix(body, "//")
		if body == ":memory:" || body == "" {
			return ":memory:?" + pragma, nil
		}
		return combineFileDSN("file:"+body, pragma), nil

	case strings.HasPrefix(dsn, "file:"):
		return combineFileDSN(dsn, pragma), nil

	default:
		// Plain filesystem path — wrap with `file:` and pragmas.
		return combineFileDSN("file:"+dsn, pragma), nil
	}
}

// combineFileDSN appends `_pragma=…` parameters to a `file:` DSN without
// clobbering any pragmas the caller already supplied. The modernc.org/sqlite
// driver reads multiple `_pragma=` occurrences and applies each on the
// fresh connection.
func combineFileDSN(fileDSN, pragma string) string {
	if strings.Contains(fileDSN, "?") {
		return fileDSN + "&" + pragma + "&_pragma=journal_mode(wal)"
	}
	return fileDSN + "?" + pragma + "&_pragma=journal_mode(wal)"
}

// ApplyMigrations runs the bundled migrations against the underlying
// connection. The embedded migration file is plain SQLite SQL (no goose
// directives), so we execute it as a single batch. The migrations are
// idempotent (CREATE TABLE / CREATE INDEX use IF NOT EXISTS), so calling
// ApplyMigrations on an already-migrated database is a no-op.
func (s *Store) ApplyMigrations(ctx context.Context) error {
	body, err := embedMigrations.ReadFile("migrations/0001.sql")
	if err != nil {
		return fmt.Errorf("sqlite: read embedded migration: %w", err)
	}

	if _, err := s.DB.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("sqlite: apply migration: %w", err)
	}

	return nil
}
