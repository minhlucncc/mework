package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// RunnerIdentity is the SQLite analogue of the runner_identity row used
// by libs/server/auth to validate rt_token lookups.
type RunnerIdentity struct {
	ID         string
	TokenHash  string
	AccountID  string
	RuntimeID  string
	Status     string
	LastSeenAt sql.NullTime
}

// InsertRunnerIdentity persists the row backing a runtime's rt_token.
// TokenHash is the HMAC lookup of the plaintext rt_token; the plaintext
// never reaches the store (matches the auth-and-secrets invariant).
func (s *Store) InsertRunnerIdentity(ctx context.Context, id, tokenHash, accountID, runtimeID, status string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO runner_identity (id, rt_token_hash, account_id, runtime_id, status)
		VALUES (?, ?, ?, ?, ?)
	`, id, tokenHash, accountID, runtimeID, status)
	if err != nil {
		return fmt.Errorf("sqlite: insert runner_identity %q: %w", id, err)
	}
	return nil
}

// GetRunnerIdentity reads a runner row by the HMAC token lookup. Returns
// (nil, nil) when not found.
func (s *Store) GetRunnerIdentity(ctx context.Context, tokenHash string) (*RunnerIdentity, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, rt_token_hash, account_id, runtime_id, status, last_seen_at
		FROM runner_identity WHERE rt_token_hash = ?
	`, tokenHash)
	var r RunnerIdentity
	if err := row.Scan(&r.ID, &r.TokenHash, &r.AccountID, &r.RuntimeID, &r.Status, &r.LastSeenAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: get runner_identity: %w", err)
	}
	return &r, nil
}
