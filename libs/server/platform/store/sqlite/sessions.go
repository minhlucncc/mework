package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// Session mirrors the rows of the `sessions` table. The store uses it to
// bind daemon sessions to their owning runtime.
type Session struct {
	ID        string
	RuntimeID string
	Status    string
}

// InsertSession persists a session row keyed by id with the given runtime
// binding and status. Caller is responsible for having inserted the
// runtime row first (FK target).
func (s *Store) InsertSession(ctx context.Context, id, runtimeID, status string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO sessions (id, runtime_id, status)
		VALUES (?, ?, ?)
	`, id, runtimeID, status)
	if err != nil {
		return fmt.Errorf("sqlite: insert session %q: %w", id, err)
	}
	return nil
}

// GetSession reads a single session row by id.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, runtime_id, status
		FROM sessions WHERE id = ?
	`, id)
	var sess Session
	err := row.Scan(&sess.ID, &sess.RuntimeID, &sess.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get session %q: %w", id, err)
	}
	return &sess, nil
}
