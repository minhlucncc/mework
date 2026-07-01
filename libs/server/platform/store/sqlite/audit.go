package sqlite

import (
	"context"
	"fmt"
)

// AuditEntry is the SQLite analogue of an audit_log row. The store layer
// writes entries via Append; reads are done by callers with their own
// SQL because the audit_log table is not the focus of unit 01.
type AuditEntry struct {
	ID         string
	TenantID   string
	ActorID    string
	ActorType  string
	Action     string
	TargetType string
	TargetID   string
	Metadata   []byte
}

// Append writes one audit entry. Returns the new id (caller-supplied or
// generated-by-store semantics: in this offline driver the caller must
// supply id to keep the row immutable and easy to export).
func (s *Store) Append(ctx context.Context, e AuditEntry) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO audit_log (
			id, tenant_id, actor_id, actor_type, action, target_type, target_id, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.TenantID, e.ActorID, e.ActorType, e.Action, e.TargetType, e.TargetID, string(e.Metadata))
	if err != nil {
		return fmt.Errorf("sqlite: append audit entry %q: %w", e.ID, err)
	}
	return nil
}
