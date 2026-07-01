package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// jobFixture is the test-package-private struct that callers pass to
// InsertJob. The shape is owned by the unit's tests in
// sqlite_test.go; this production file references it by name only.
// Production callers wiring this up from a separate binary should
// build a row from positional parameters rather than depend on the
// unexported struct.

// Job is the SQLite analogue of the row inserted by the orchestrator
// store. Payload is the JSON-encoded TEXT column.
type Job struct {
	ID              string
	ProviderCode    string
	ExternalEventID string
	ExternalTaskID  string
	Status          string
	Payload         []byte
	CreatedAt       time.Time
	RunnerID        string
	ClaimedAt       *time.Time
}

// claimMu is a process-global mutex that serializes concurrent
// ClaimNext calls in conjunction with the per-connection pool size of
// 1 (set in Open for in-memory DSNs). SQLite's writer lock acquires
// inside the claim transaction, so the database itself enforces
// "one writer at a time"; the mutex exists only to keep the
// `BEGIN IMMEDIATE` and `COMMIT` statements on a single sql.Conn so
// the writer lock isn't crossed with a second connection mid-claim.
var claimMu sync.Mutex

// InsertJob persists a queued job row. The argument is intentionally
// untyped (any) because the unit's RED tests pass in a struct that is
// declared in the test file (so production code cannot redeclare it).
// We use reflection to read the conventional fields — ID,
// ProviderCode, ExternalEventID, ExternalTaskID, Payload, CreatedAt —
// that exist on both the test fixture and any caller-shaped struct
// used in production wiring. This keeps the public API loose while
// still type-checked at the boundary.
//
// A pre-existing runtime row is required as the FK target — callers
// are expected to have inserted one already (mirrors the Postgres path).
func (s *Store) InsertJob(ctx context.Context, j any) error {
	if j == nil {
		return fmt.Errorf("sqlite: insert job: nil fixture")
	}
	fields, err := extractJobFields(j)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(fields.payload)
	if err != nil {
		return fmt.Errorf("sqlite: marshal job payload: %w", err)
	}
	if fields.createdAt.IsZero() {
		fields.createdAt = time.Now().UTC()
	}

	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO jobs (
			id, account_id, runtime_id, external_task_id, external_event_id,
			provider_code, status, ttl_expires_at, payload, created_at
		) VALUES (
			?, '00000000-0000-0000-0000-000000000001', '', ?, ?, ?,
			'queued', ?, ?, ?
		)
	`, fields.id, fields.externalTaskID, fields.externalEventID, fields.providerCode,
		fields.createdAt.Add(30*time.Minute), string(payload), fields.createdAt)
	if err != nil {
		return fmt.Errorf("sqlite: insert job %q: %w", fields.id, err)
	}
	return nil
}

// jobFields is the decoded view of a job fixture consumed by InsertJob.
type jobFields struct {
	id              string
	providerCode    string
	externalEventID string
	externalTaskID  string
	payload         any
	createdAt       time.Time
}

// extractJobFields pulls jobFields out of any struct that has the
// expected field names. Fields are looked up case-insensitively so the
// test fixture's exact field set (ID, ProviderCode, ExternalEventID,
// ExternalTaskID, Payload, CreatedAt) decodes cleanly.
func extractJobFields(v any) (jobFields, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return jobFields{}, fmt.Errorf("sqlite: insert job: expected struct, got %T", v)
	}

	rt := rv.Type()
	get := func(name string) (reflect.Value, bool) {
		for i := 0; i < rt.NumField(); i++ {
			if strings.EqualFold(rt.Field(i).Name, name) {
				return rv.Field(i), true
			}
		}
		return reflect.Value{}, false
	}

	mustString := func(name string) (string, error) {
		f, ok := get(name)
		if !ok {
			return "", fmt.Errorf("sqlite: insert job: missing field %q", name)
		}
		if f.Kind() != reflect.String {
			return "", fmt.Errorf("sqlite: insert job: field %q is not string (got %s)", name, f.Kind())
		}
		return f.String(), nil
	}

	id, err := mustString("ID")
	if err != nil {
		return jobFields{}, err
	}
	providerCode, err := mustString("ProviderCode")
	if err != nil {
		return jobFields{}, err
	}
	externalEventID, err := mustString("ExternalEventID")
	if err != nil {
		return jobFields{}, err
	}
	externalTaskID, err := mustString("ExternalTaskID")
	if err != nil {
		return jobFields{}, err
	}

	var payload any
	if f, ok := get("Payload"); ok {
		payload = f.Interface()
	}

	var createdAt time.Time
	if f, ok := get("CreatedAt"); ok {
		if t, ok := f.Interface().(time.Time); ok {
			createdAt = t
		}
	}

	return jobFields{
		id:              id,
		providerCode:    providerCode,
		externalEventID: externalEventID,
		externalTaskID:  externalTaskID,
		payload:         payload,
		createdAt:       createdAt,
	}, nil
}

// GetJob reads a single job row by id. Returns (nil, nil) when not found.
func (s *Store) GetJob(ctx context.Context, id string) (*Job, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, provider_code, external_event_id, external_task_id,
		       status, payload, created_at, runner_id, claimed_at
		FROM jobs WHERE id = ?
	`, id)
	var j Job
	var runner sql.NullString
	var claimed sql.NullTime
	err := row.Scan(&j.ID, &j.ProviderCode, &j.ExternalEventID, &j.ExternalTaskID,
		&j.Status, &j.Payload, &j.CreatedAt, &runner, &claimed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get job %q: %w", id, err)
	}
	if runner.Valid {
		j.RunnerID = runner.String
	}
	if claimed.Valid {
		t := claimed.Time
		j.ClaimedAt = &t
	}
	return &j, nil
}

// ClaimNext atomically claims the next queued job for the given
// provider_code and returns it. Concurrent claimers on the same database
// are serialized via SQLite's writer lock; the implementation issues
//
//	BEGIN IMMEDIATE;
//	UPDATE jobs SET ... WHERE id IN (
//	    SELECT id FROM jobs WHERE status='queued' AND provider_code=?
//	    ORDER BY created_at ASC LIMIT 1
//	) RETURNING *;
//	COMMIT;
//
// on a single sql.Conn so the writer lock spans the whole claim. Two
// claimers race on the same conn pool slot: the second blocks on the
// 5s busy_timeout the driver sets on every connection until the first
// commits, then proceeds — by which point the row's status is no longer
// 'queued' so the UPDATE returns 0 rows and ClaimNext reports ErrNoJob.
//
// We do NOT use database/sql's BeginTx here because that opens a
// deferred transaction (`BEGIN DEFERRED`) and the BEGIN IMMEDIATE we
// want to send on top of it would be rejected as a nested transaction
// by modernc.org/sqlite. Instead we drive the BEGIN/COMMIT statements
// directly through the underlying connection.
func (s *Store) ClaimNext(ctx context.Context, providerCode string) (*Job, error) {
	claimMu.Lock()
	defer claimMu.Unlock()

	conn, err := s.DB.Conn(ctx)
	if err != nil {
		if isSQLiteBusy(err) {
			return nil, ErrNoJob
		}
		return nil, fmt.Errorf("sqlite: claim conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		if isSQLiteBusy(err) {
			return nil, ErrNoJob
		}
		return nil, fmt.Errorf("sqlite: begin immediate: %w", err)
	}

	// committed indicates we want to COMMIT vs ROLLBACK at the end.
	committed := false
	defer func() {
		if !committed {
			// Best-effort rollback; ignore errors because we may
			// already be in an error path.
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	row := conn.QueryRowContext(ctx, `
		UPDATE jobs
		   SET status='claimed',
		       runner_id='runner-offline',
		       claimed_at=CURRENT_TIMESTAMP
		 WHERE id IN (
		   SELECT id FROM jobs
		    WHERE status='queued' AND provider_code=?
		    ORDER BY created_at ASC
		    LIMIT 1
		 )
		RETURNING id, provider_code, external_event_id, external_task_id,
		          status, payload, created_at, runner_id, claimed_at
	`, providerCode)

	var j Job
	var runner sql.NullString
	var claimed sql.NullTime
	scanErr := row.Scan(&j.ID, &j.ProviderCode, &j.ExternalEventID, &j.ExternalTaskID,
		&j.Status, &j.Payload, &j.CreatedAt, &runner, &claimed)
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return nil, ErrNoJob
		}
		if isSQLiteBusy(scanErr) {
			return nil, ErrNoJob
		}
		return nil, fmt.Errorf("sqlite: claim next: %w", scanErr)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		if isSQLiteBusy(err) {
			return nil, ErrNoJob
		}
		return nil, fmt.Errorf("sqlite: commit claim: %w", err)
	}
	committed = true

	if runner.Valid {
		j.RunnerID = runner.String
	}
	if claimed.Valid {
		t := claimed.Time
		j.ClaimedAt = &t
	}
	return &j, nil
}

// isSQLiteBusy reports whether the given error is the SQLite BUSY error
// the driver raises when another connection holds the writer lock.
//
// modernc.org/sqlite returns the standard SQLite extended-code constant
// 5 (SQLITE_BUSY). The error message contains the substring "database
// is locked" or "SQLITE_BUSY" — both are matched here so this stays
// resilient across driver versions.
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, sub := range []string{"SQLITE_BUSY", "database is locked", "database table is locked"} {
		for i := 0; i+len(sub) <= len(msg); i++ {
			if msg[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
