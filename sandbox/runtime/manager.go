// Package runtime manages the lifecycle of a sandbox environment: start, stop,
// hooks, workspace mount, and teardown. It delegates to the active engine
// via the sandbox driver interface from shared/ports.
package runtime

import (
	"context"
	"fmt"

	"mework/shared/ports"
)

// Manager manages a sandbox environment through its lifecycle.
type Manager struct {
	engine ports.SandboxDriver
}

// NewManager creates a new sandbox Manager with the given engine.
func NewManager(driver ports.SandboxDriver) *Manager {
	return &Manager{engine: driver}
}

// Start starts a sandbox environment.
func (m *Manager) Start(ctx context.Context) error {
	return fmt.Errorf("sandbox runtime Start: not implemented")
}

// Stop stops a sandbox environment.
func (m *Manager) Stop(ctx context.Context) error {
	return fmt.Errorf("sandbox runtime Stop: not implemented")
}

// Hooks returns the lifecycle hooks supported by this sandbox.
func (m *Manager) Hooks() []string {
	return nil
}

// WorkspaceMount returns the path where the workspace is mounted.
func (m *Manager) WorkspaceMount() string {
	return ""
}

// Teardown cleans up the sandbox environment.
func (m *Manager) Teardown(ctx context.Context) error {
	return fmt.Errorf("sandbox runtime Teardown: not implemented")
}
