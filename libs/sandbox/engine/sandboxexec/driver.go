// Package sandboxexec implements the ports.SandboxDriver interface using
// macOS sandbox-exec with built-in Seatbelt profiles for process isolation.
//
// This is the platform-appropriate hardening for the "local" engine on macOS.
// It wraps subprocesses in sandbox-exec -n no-write, which prevents writes
// outside the working directory (like /etc, /Users, and system paths).
//
// NOTE: On macOS 15+, custom Seatbelt profiles (via -f or -p) fail with
// "Operation not permitted" when they include (deny default) — this is an
// OS-level restriction on CLI sandbox-exec. Built-in profiles (-n) work.
// The no-write profile provides meaningful protection against the primary
// threat: malicious prompt content modifying host files.
//
// Availability: macOS only. On non-macOS systems, New() returns nil.
package sandboxexec

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// driverProfileName is the built-in Seatbelt profile to use.
// "no-write" prevents filesystem writes outside of allowed paths.
const driverProfileName = "no-write"

// Driver implements ports.SandboxDriver using macOS sandbox-exec.
type Driver struct{}

// New creates a new sandbox-exec Driver. Returns nil if sandbox-exec is not
// available (non-macOS or missing binary).
func New() *Driver {
	if runtime.GOOS != "darwin" {
		return nil
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return nil
	}
	return &Driver{}
}

// Caps returns the capabilities of the sandbox-exec driver.
func (d *Driver) Caps() core.SandboxCaps {
	return core.SandboxCaps{
		IsIsolated:  true,
		IsRemote:    false,
		SupportsGPU: true,
		SupportsNet: true,
		MaxMemoryMB: 0,
		MaxDiskMB:   0,
		DriverName:  "sandbox-exec",
	}
}

// sandboxExecSandbox is a running sandbox-exec sandbox.
type sandboxExecSandbox struct {
	id          string
	workDir     string
	profileName string
}

func (s *sandboxExecSandbox) ID() string { return s.id }

// Exec runs a command inside the sandbox-exec sandbox.
func (s *sandboxExecSandbox) Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(command) == 0 {
		return -1, fmt.Errorf("command is required")
	}

	// sandbox-exec -n <profile> -- command...
	sbArgs := []string{
		"-n", s.profileName,
		"--",
	}
	sbArgs = append(sbArgs, command...)

	cmd := exec.CommandContext(ctx, "sandbox-exec", sbArgs...)
	cmd.Dir = s.workDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
		}
		return -1, err
	}
	return 0, nil
}

// Mount creates a symlink to the workspace inside the workdir. Since
// sandbox-exec shares the host filesystem (only restricts writes), no
// real mount is needed.
func (s *sandboxExecSandbox) Mount(ctx context.Context, workspace core.Workspace, targetPath string) error {
	if workspace.Path == "" {
		return nil
	}
	if targetPath == "" {
		targetPath = filepath.Base(workspace.Path)
	}
	target := filepath.Join(s.workDir, targetPath)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing mount target: %w", err)
	}
	return os.Symlink(workspace.Path, target)
}

// Signals is not supported for sandbox-exec (process is signalled via context).
func (s *sandboxExecSandbox) Signals(ctx context.Context, sig string) error {
	return nil
}

// Start creates a new sandbox-exec sandbox.
func (d *Driver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	workDir := spec.SandboxID
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "mework-sandbox-exec", spec.AgentID)
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}

	return &sandboxExecSandbox{
		id:          spec.SandboxID,
		workDir:     workDir,
		profileName: driverProfileName,
	}, nil
}

// Stop is a no-op for sandbox-exec.
func (d *Driver) Stop(ctx context.Context, sandboxID string) error { return nil }

// Destroy is a no-op for sandbox-exec.
func (d *Driver) Destroy(ctx context.Context, sandboxID string) error { return nil }
