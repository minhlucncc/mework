// Package storage defines the object-store interface for server-side blob
// storage. It wraps the ObjectStore port from shared/ports.
//
// This is a stub — the full implementation lands in a downstream change.
package storage

import (
	"context"
	"io"

	"mework/shared/core"
)

// Store is the server's object storage interface.
type Store interface {
	Put(ctx context.Context, ref core.ObjectRef, reader io.Reader) error
	Get(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error)
	Delete(ctx context.Context, ref core.ObjectRef) error
	List(ctx context.Context, prefix string) ([]core.ObjectInfo, error)
}
