package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Profile is the SQLite analogue of the profiles row used by the agent
// catalog / registry. Payload is stored as a JSON-encoded TEXT column;
// callers pass any Go value and it is marshalled here.
type Profile struct {
	ID      string
	Account string
	Name    string
}

// InsertProfile persists a profile with its payload as a JSON-encoded TEXT
// column. account_id is set to a fixed default tenant ("00000000-0000-0000-0000-000000000001")
// to mirror the Postgres default and keep the FK targets satisfied.
func (s *Store) InsertProfile(ctx context.Context, id, name string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sqlite: marshal profile payload: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO profiles (id, account_id, name, body, workflow_config)
		VALUES (?, '00000000-0000-0000-0000-000000000001', ?, ?, '{}')
	`, id, name, string(body))
	if err != nil {
		return fmt.Errorf("sqlite: insert profile %q: %w", id, err)
	}
	return nil
}

// GetProfilePayload returns the JSON-encoded body column for the profile
// identified by id. Callers json.Unmarshal the bytes into their own type
// — the store does not assume a struct shape.
func (s *Store) GetProfilePayload(ctx context.Context, id string) ([]byte, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT body FROM profiles WHERE id = ?`, id)
	var body []byte
	if err := row.Scan(&body); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: get profile payload %q: %w", id, err)
	}
	return body, nil
}
