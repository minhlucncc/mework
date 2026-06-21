// Package workspacefs provides a local filesystem implementation of the
// ObjectStore interface from shared/ports, storing workspace files on disk.
//
// This is a stub — the full implementation lands in a downstream change.
package workspacefs

import (
	"context"
	"fmt"
	"io"

	"mework/shared/core"
)

// WorkspaceFS is a local-filesystem-backed object store for workspace files.
type WorkspaceFS struct{}

// New creates a new WorkspaceFS instance.
func New() *WorkspaceFS {
	return &WorkspaceFS{}
}

// Put stores a file in the workspace.
func (w *WorkspaceFS) Put(ctx context.Context, ref core.ObjectRef, reader io.Reader) error {
	return fmt.Errorf("WorkspaceFS.Put: not implemented")
}

// Get retrieves a file from the workspace.
func (w *WorkspaceFS) Get(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error) {
	return nil, fmt.Errorf("WorkspaceFS.Get: not implemented")
}

// Delete removes a file from the workspace.
func (w *WorkspaceFS) Delete(ctx context.Context, ref core.ObjectRef) error {
	return fmt.Errorf("WorkspaceFS.Delete: not implemented")
}

// List returns files matching a prefix.
func (w *WorkspaceFS) List(ctx context.Context, prefix string) ([]core.ObjectInfo, error) {
	return nil, fmt.Errorf("WorkspaceFS.List: not implemented")
}
