package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// Runtime mirrors the columns of the `runtimes` table that callers care
// about for the offline store. It is the SQLite analogue of the row used
// by the Postgres implementation in libs/server/registry/.
//
// Times are stored as TIMESTAMP (TEXT in modernc.org/sqlite) and decoded
// with sql.NullString so callers can distinguish "never seen" from a
// concrete timestamp.
type Runtime struct {
	ID          string
	AccountID   string
	Code        string
	Label       string
	TokenLookup string
	Status      string
}

// InsertRuntime inserts a single runtime row. The columns selected are the
// minimum needed to satisfy the round-trip test; additional columns fall
// back to their declared defaults (created_at=NOW, last_seen_at=NULL).
func (s *Store) InsertRuntime(ctx context.Context, id, accountID, code, label, tokenLookup, status string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO runtimes (id, account_id, code, label, token_lookup, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, accountID, code, label, tokenLookup, status)
	if err != nil {
		return fmt.Errorf("sqlite: insert runtime %q: %w", id, err)
	}
	return nil
}

// GetRuntime reads a single runtime row by id.
func (s *Store) GetRuntime(ctx context.Context, id string) (*Runtime, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, account_id, code, label, token_lookup, status
		FROM runtimes WHERE id = ?
	`, id)
	var r Runtime
	err := row.Scan(&r.ID, &r.AccountID, &r.Code, &r.Label, &r.TokenLookup, &r.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get runtime %q: %w", id, err)
	}
	return &r, nil
}
