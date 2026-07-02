// Package runtime manages the lifecycle of a sandbox environment: start, stop,
// hooks, workspace mount, and teardown. It delegates to the active engine
// via the sandbox driver interface from shared/ports.
package runtime

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"sync"

	"mework/libs/sandbox/engine/cloudflare"
	"mework/libs/sandbox/engine/custom"
	"mework/libs/sandbox/engine/docker"
	"mework/libs/sandbox/engine/local"
	"mework/libs/shared/core"
	"mework/libs/shared/policy"
	"mework/libs/shared/ports"
)

// ManagerConfig configures a sandbox Manager.
type ManagerConfig struct {
	// MaxSandboxes limits concurrent sandboxes. 0 = unlimited. Default 10.
	MaxSandboxes int
	// Logger receives structured audit events. If nil, no audit logging.
	Logger *slog.Logger
	// SecretInjector, if set, is called after Start + Mount to inject secrets.
	SecretInjector ports.SecretInjector
	// Policy, if set, is enforced before each Start and Exec call.
	Policy *policy.Policy
}

// DefaultManagerConfig is the default manager configuration.
var DefaultManagerConfig = ManagerConfig{
	MaxSandboxes: 10,
}

// Manager manages a sandbox environment through its lifecycle.
type Manager struct {
	mu      sync.Mutex
	driver  ports.SandboxDriver
	running map[string]ports.Sandbox // active sandboxes keyed by ID

	cfg ManagerConfig
}

// NewManager creates a new sandbox Manager with the given engine and config.
func NewManager(driver ports.SandboxDriver, config ...ManagerConfig) *Manager {
	cfg := DefaultManagerConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	// Warn if the driver reports no isolation.
	caps := driver.Caps()
	if !caps.IsIsolated {
		log.Printf("WARNING: sandbox engine %q reports NO host isolation — use only for trusted agents", caps.DriverName)
	}

	return &Manager{
		driver:  driver,
		running: make(map[string]ports.Sandbox),
		cfg:     cfg,
	}
}

// NewManagerFor creates a Manager for the named engine ("local", "docker",
// "cloudflare", "custom"). Returns an error for unknown or empty engine names.
func NewManagerFor(engine string) (*Manager, error) {
	switch engine {
	case "docker":
		return NewManager(docker.New()), nil
	case "cloudflare":
		return NewManager(cloudflare.New()), nil
	case "custom":
		d, err := custom.New()
		if err != nil {
			return nil, fmt.Errorf("custom engine unavailable: %w", err)
		}
		return NewManager(d), nil
	case "local":
		return NewManager(local.New()), nil
	default:
		return nil, fmt.Errorf("unknown sandbox engine %q; valid: local, docker, cloudflare, custom", engine)
	}
}

// Caps returns the capabilities of the managed driver.
func (m *Manager) Caps() core.SandboxCaps {
	return m.driver.Caps()
}

// Start creates and starts a new sandbox for the given spec.
func (m *Manager) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Enforce one agent per sandbox: check for duplicate ID.
	if _, exists := m.running[spec.SandboxID]; exists {
		return nil, fmt.Errorf("sandbox %q already exists", spec.SandboxID)
	}

	// Enforce sandbox count limit.
	maxSbx := m.cfg.MaxSandboxes
	if maxSbx > 0 && len(m.running) >= maxSbx {
		return nil, fmt.Errorf("sandbox limit reached: %d/%d", len(m.running), maxSbx)
	}

	if m.cfg.Policy != nil {
		attrs := policy.Attributes{
			"action":    "start",
			"engine":    m.driver.Caps().DriverName,
			"agent_id":  spec.AgentID,
			"backend":   spec.BackendName,
			"sandbox_id": spec.SandboxID,
		}
		result, err := m.cfg.Policy.Enforce(attrs)
		if err != nil {
			return nil, fmt.Errorf("policy evaluation error: %w", err)
		}
		if !result.Allowed {
			return nil, fmt.Errorf("policy denied: %s", result.Reason)
		}
	}

	// Capability enforcement: check if the spec requires features the driver lacks.
	caps := m.driver.Caps()
	if spec.RequiresNet && !caps.SupportsNet {
		return nil, fmt.Errorf("engine %q does not support networking (required by spec)", caps.DriverName)
	}
	if spec.RequiresGPU && !caps.SupportsGPU {
		return nil, fmt.Errorf("engine %q does not support GPU (required by spec)", caps.DriverName)
	}

	s, err := m.driver.Start(ctx, spec)
	if err != nil {
		return nil, err
	}

	// Mount the bound workspace into the sandbox. The local engine binds its
	// workdir directly and leaves Workspace.Path empty, so an unbound spec is a
	// no-op here (the unbound path is unchanged).
	if spec.Workspace.Path != "" {
		if err := s.Mount(ctx, spec.Workspace, m.WorkspaceMount()); err != nil {
			// Clean up the just-started sandbox rather than leaking it.
			_ = m.driver.Destroy(ctx, s.ID())
			return nil, fmt.Errorf("mount workspace into sandbox %q: %w", s.ID(), err)
		}
	}

	m.running[s.ID()] = s
	m.audit("sandbox_started", "engine", caps.DriverName, "sandbox_id", s.ID(), "agent", spec.AgentID)
	return s, nil
}

// Stop stops a running sandbox gracefully.
func (m *Manager) Stop(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.running[sandboxID]
	if !exists {
		return fmt.Errorf("sandbox %q not found", sandboxID)
	}

	if err := m.driver.Stop(ctx, sandboxID); err != nil {
		return err
	}

	delete(m.running, sandboxID)
	m.audit("sandbox_stopped", "sandbox_id", sandboxID)
	return nil
}

// Destroy forcibly removes a sandbox and its resources.
func (m *Manager) Destroy(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.driver.Destroy(ctx, sandboxID); err != nil {
		return err
	}

	delete(m.running, sandboxID)
	m.audit("sandbox_destroyed", "sandbox_id", sandboxID)
	return nil
}

// DestroyAll stops and removes all active sandboxes.
func (m *Manager) DestroyAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for id := range m.running {
		if err := m.driver.Destroy(ctx, id); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
		delete(m.running, id)
	}
	if firstErr == nil {
		m.audit("sandbox_all_destroyed")
	}
	return firstErr
}

// Hooks returns the lifecycle hooks supported by this sandbox.
func (m *Manager) Hooks() []string {
	return nil
}

// WorkspaceMount returns the path where the bound workspace is mounted inside
// the sandbox.
func (m *Manager) WorkspaceMount() string {
	return "/workspace"
}

// Teardown cleans up all sandbox environments.
func (m *Manager) Teardown(ctx context.Context) error {
	return m.DestroyAll(ctx)
}

// ActiveCount returns the number of currently running sandboxes.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.running)
}

// audit logs a structured event if a logger is configured.
func (m *Manager) audit(event string, attrs ...any) {
	if m.cfg.Logger != nil {
		m.cfg.Logger.Info(event, attrs...)
	}
}
