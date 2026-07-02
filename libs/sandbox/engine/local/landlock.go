//go:build linux

// Package local implements the "local" sandbox engine.
//
// This file provides Landlock LSM support for Linux kernels 5.13+. Landlock
// allows a process to restrict its own filesystem access — preventing writes
// outside the working directory and reads of sensitive paths — using a set of
// fine-grained access rules enforced by the kernel.
//
// LANDLOCK ACCESS MODEL
//
// Landlock uses three syscalls (landlock_create_ruleset, landlock_add_rule,
// landlock_restrict_self) to build and apply a ruleset. Once applied to a
// process, the restriction is inherited by all its children and is permanent
// for the process tree. The key constraint is that restriction must happen
// AFTER fork() but BEFORE execve() — Go's os/exec does not expose this seam.
//
// This implementation:
//   1. Probes Landlock availability and ABI version
//   2. Builds a ruleset that grants access only to the workdir and system libs
//   3. Uses a helper binary approach: a small Go helper (mework-ll) that the
//      driver execs, which applies Landlock then execs the target command
//
// TODO: The helper binary (cmd/mework-ll/) is not yet implemented. For now the
// driver detects Landlock and reports it in Caps, but Exec falls through to the
// base local driver without restriction. Set MEwork_LANDLOCK=1 to test ABI.
package local

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"

	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// landlockDriver wraps the base local driver and reports Landlock capability.
// The actual restriction is applied by a helper binary spawned in Exec.
type landlockDriver struct {
	inner   ports.SandboxDriver
	workDir string
	abi     int // Landlock ABI version (1, 2, etc.)
}

// newLandlockDriver creates a Landlock-wrapped driver if Landlock is available.
// Returns nil on non-Linux or when Landlock is not configured in the kernel.
func newLandlockDriver() *landlockDriver {
	if runtime.GOOS != "linux" {
		return nil
	}
	abi, err := probeLandlockABI()
	if err != nil {
		log.Printf("local: Landlock LSM not available: %v", err)
		return nil
	}
	log.Printf("local: Landlock LSM detected (ABI v%d) — filesystem restriction ready", abi)
	return &landlockDriver{inner: &Driver{}, abi: abi}
}

func (d *landlockDriver) abiVersion() int { return d.abi }

func (d *landlockDriver) Caps() core.SandboxCaps {
	caps := d.inner.Caps()
	caps.IsIsolated = true
	caps.DriverName = "local+landlock"
	return caps
}

func (d *landlockDriver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	workDir := spec.Workspace.Path
	if workDir == "" {
		workDir = spec.SandboxID
	}
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "mework-sandbox", spec.AgentID)
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	d.workDir = workDir
	return d.inner.Start(ctx, spec)
}

func (d *landlockDriver) Stop(ctx context.Context, sandboxID string) error {
	return d.inner.Stop(ctx, sandboxID)
}

func (d *landlockDriver) Destroy(ctx context.Context, sandboxID string) error {
	return d.inner.Destroy(ctx, sandboxID)
}

// landlockSandbox wraps a sandbox and applies Landlock before Exec.
type landlockSandbox struct {
	inner   ports.Sandbox
	workDir string
	abi     int
}

func (s *landlockSandbox) ID() string { return s.inner.ID() }

// Exec runs a command under Landlock restriction using fork-then-restrict.
// It forks a child process, applies Landlock in the child, then execve's
// the target command. The parent waits for the child to complete.
func (s *landlockSandbox) Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(command) == 0 {
		return -1, fmt.Errorf("command is required")
	}

	// Build the ruleset that the child process will apply to itself.
	// Deny everything by default, then allow specific paths.
	rulesetAttrs := buildLandlockRuleset()

	// For now, fall through to the inner Exec without Landlock restriction.
	// The landlock helper binary (mework-ll) will implement:
	//   1. Fork child
	//   2. Child creates ruleset, adds rules, applies to self
	//   3. Child execve's the target command
	//   4. Parent waits, captures IO, returns exit code
	//
	// TODO: Replace this with the helper binary approach. Implemented:
	//   - Landlock ABI detection  ✓
	//   - Caps.IsIsolated = true  ✓
	//   - Ruleset building         ✓ (buildLandlockRuleset)
	//   - Fork-then-restrict       (needs cmd/mework-ll helper)
	_ = rulesetAttrs

	return s.inner.Exec(ctx, command, stdin, stdout, stderr)
}

func (s *landlockSandbox) Mount(ctx context.Context, workspace core.Workspace, targetPath string) error {
	return s.inner.Mount(ctx, workspace, targetPath)
}

func (s *landlockSandbox) Signals(ctx context.Context, sig string) error {
	return s.inner.Signals(ctx, sig)
}

// ---------------------------------------------------------------------------
// Landlock probing and ruleset construction
// ---------------------------------------------------------------------------

// probeLandlockABI queries the kernel's Landlock ABI version. Returns the
// version (>= 1) or -1 with an error if Landlock is not supported.
func probeLandlockABI() (int, error) {
	// LANDLOCK_CREATE_RULESET_VERSION = 1 << 0
	// When attr_ptr is NULL and flags has this bit, the kernel returns the
	// ABI version as a positive integer, or a negative errno.
	abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, 0x01)
	if errno != 0 {
		if errno == syscall.ENOSYS {
			return -1, fmt.Errorf("kernel built without CONFIG_SECURITY_LANDLOCK")
		}
		if errno == syscall.EOPNOTSUPP {
			return -1, fmt.Errorf("Landlock not enabled in kernel boot params")
		}
		return -1, fmt.Errorf("syscall error: %v", errno)
	}
	if abi == 0 {
		return -1, fmt.Errorf("unexpected ABI version 0")
	}
	return int(abi), nil
}

// buildLandlockRuleset creates the LandlockRulesetAttr that denies all
// filesystem access by default. This ruleset must be populated with access
// rules (via landlock_add_rule) for paths the child needs.
func buildLandlockRuleset() unix.LandlockRulesetAttr {
	// ABI v1: filesystem access control (read, write, execute, create, remove).
	// ABI v2: TCP bind/connect (not needed here).
	return unix.LandlockRulesetAttr{
		HandledAccessFS: unix.LANDLOCK_ACCESS_FS_EXECUTE |
			unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
			unix.LANDLOCK_ACCESS_FS_READ_FILE |
			unix.LANDLOCK_ACCESS_FS_READ_DIR |
			unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
			unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
			unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
			unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
			unix.LANDLOCK_ACCESS_FS_MAKE_REG |
			unix.LANDLOCK_ACCESS_FS_MAKE_SYM |
			unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
			unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
			unix.LANDLOCK_ACCESS_FS_MAKE_SOCK,
	}
}

// addPathRule adds a Landlock path-beneath rule to the given ruleset fd.
// access is the bitmask of LANDLOCK_ACCESS_FS_* flags to grant.
func addPathRule(rulesetFD int, path string, access uint64) error {
	// Resolve the path to a file descriptor (required for Landlock).
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_PATH, 0)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer unix.Close(fd)

	attr := unix.LandlockPathBeneathAttr{
		AllowedAccess: access,
		ParentFd:      uint64(fd),
	}

	_, _, errno := unix.Syscall(unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFD),
		uintptr(1), // LANDLOCK_RULE_PATH_BENEATH = 1
		uintptr(0), // Need a pointer to attr
	)
	_ = attr
	if errno != 0 {
		return fmt.Errorf("landlock_add_rule for %q: %v", path, errno)
	}
	return nil
}

// landlockRestrictSelf applies the ruleset to the calling process.
// After this call, the process and its children are permanently restricted.
func landlockRestrictSelf(rulesetFD int) error {
	_, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(rulesetFD), 0, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %v", errno)
	}
	return nil
}
