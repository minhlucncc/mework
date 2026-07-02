// Package docker implements the ports.SandboxDriver interface for Docker
// containers. Each agent runs in its own container for process isolation.
// The driver uses the docker CLI (via exec) rather than the Docker SDK so
// that local-only builds add no third-party dependency.
package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// Driver implements ports.SandboxDriver for Docker containers.
type Driver struct{}

// New creates a new Docker Driver.
func New() *Driver { return &Driver{} }

// Caps returns the capabilities of this driver.
func (d *Driver) Caps() core.SandboxCaps {
	return core.SandboxCaps{
		IsIsolated:  true,
		IsRemote:    false,
		SupportsGPU: false,
		SupportsNet: false,
		MaxMemoryMB: 2048,
		MaxDiskMB:   10240,
		DriverName:  "docker",
	}
}

// dockerSandbox is a running Docker container sandbox.
type dockerSandbox struct {
	id          string
	containerID string
	workDir     string
}

func (s *dockerSandbox) ID() string { return s.id }

// Exec runs a command inside the Docker container.
func (s *dockerSandbox) Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	args := append([]string{"exec", "-i", s.containerID}, command...)
	cmd := exec.CommandContext(ctx, "docker", args...)
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

// Mount copies a source path into the container via docker cp.
func (s *dockerSandbox) Mount(ctx context.Context, workspace core.Workspace, targetPath string) error {
	source := workspace.Path
	cpCmd := exec.CommandContext(ctx, "docker", "cp", source, s.containerID+":"+targetPath)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker cp: %w\n%s", err, string(out))
	}
	return nil
}

// Signals sends a kill signal to the container.
func (s *dockerSandbox) Signals(ctx context.Context, sig string) error {
	return exec.CommandContext(ctx, "docker", "kill", "-s", sig, s.containerID).Run()
}

// resolveImage returns the image to use. Logs a warning when the image lacks
// a pinned digest (@sha256:...), since unpinned images can change silently.
func resolveImage(spec core.RunSpec) string {
	image := spec.Image
	if image == "" {
		image = "ubuntu:22.04"
	}
	if !strings.Contains(image, "@sha256:") {
		log.Printf("WARNING: image %q has no pinned digest — add @sha256:... for supply-chain safety", image)
	}
	return image
}

// Start creates and starts a Docker container for the agent run.
func (d *Driver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	image := resolveImage(spec)

	// Pull the image if not present locally.
	if err := ensureImage(ctx, image); err != nil {
		return nil, fmt.Errorf("ensure image %s: %w", image, err)
	}

	workDir := spec.SandboxID
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "mework-sandbox-docker", spec.AgentID)
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}

	containerID, err := d.createContainer(ctx, spec, image, workDir)
	if err != nil {
		return nil, err
	}

	// Start the container.
	startCmd := exec.CommandContext(ctx, "docker", "start", containerID)
	if out, err := startCmd.CombinedOutput(); err != nil {
		_ = exec.CommandContext(context.Background(), "docker", "rm", "-f", containerID).Run()
		return nil, fmt.Errorf("docker start: %w\n%s", err, string(out))
	}

	return &dockerSandbox{
		id:          spec.SandboxID,
		containerID: containerID,
		workDir:     workDir,
	}, nil
}

func (d *Driver) createContainer(ctx context.Context, spec core.RunSpec, image, workDir string) (string, error) {
	args := []string{
		"create",
		"--rm",
		"--workdir", "/work",
		"--mount", fmt.Sprintf("type=bind,source=%s,target=/work", workDir),
	}

	// === Security hardening ===

	// 1. Drop all Linux capabilities, add back only what's strictly needed.
	//    CHOWN, DAC_OVERRIDE, SETUID, SETGID, FOWNER are the minimum for
	//    package installers (npm, pip, apt).
	args = append(args,
		"--cap-drop=ALL",
		"--cap-add=CHOWN",
		"--cap-add=DAC_OVERRIDE",
		"--cap-add=SETUID",
		"--cap-add=SETGID",
		"--cap-add=FOWNER",
	)

	// 2. Prevent privilege escalation via setuid binaries.
	args = append(args, "--security-opt", "no-new-privileges:true")

	// 3. Read-only rootfs prevents writes to /etc/, /usr/, etc.
	//    Mount tmpfs for directories that need writes (/tmp, /home).
	args = append(args,
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=100m",
		"--tmpfs", "/var/tmp:rw,noexec,nosuid,size=100m",
		"--tmpfs", "/run:rw,noexec,nosuid,size=50m",
	)

	// 4. Run as non-root user for privilege de-escalation.
	args = append(args, "--user", "nobody:nogroup")

	// 5. Default resource limits — always enforced, never optional.
	memory := "512m"
	cpu := "0.5"
	pidsLimit := "100"
	if rl := spec.ResourceLimits; rl != nil {
		if rl.Memory != "" {
			memory = rl.Memory
		}
		if rl.CPU != "" {
			cpu = rl.CPU
		}
	}
	args = append(args,
		"--memory", memory,
		"--cpus", cpu,
		"--pids-limit", pidsLimit,
	)

	// 6. Apply seccomp profile (bundled allowlist) if available.
	seccompPath, seccompErr := writeSeccompProfile()
	if seccompErr == nil && seccompPath != "" {
		args = append(args, "--security-opt", "seccomp="+seccompPath)
	}
	if seccompErr != nil {
		log.Printf("WARNING: could not write seccomp profile: %v", seccompErr)
	}

	// 7. Disk storage limit from ResourceLimits.
	if rl := spec.ResourceLimits; rl != nil && rl.Disk != "" {
		if size, err := parseDiskLimit(rl.Disk); err == nil {
			args = append(args, "--storage-opt", fmt.Sprintf("size=%d", size))
		} else {
			log.Printf("WARNING: invalid disk limit %q: %v", rl.Disk, err)
		}
	}

	// 8. Network: default none, opt-in via env.
	//    When MEWORK_NETWORK=1, use Docker's internal network (no internet
	//    egress) unless MEWORK_EGRESS=1 is also set.
	hasNetwork := spec.Env["MEWORK_NETWORK"] == "1"
	allowEgress := spec.Env["MEWORK_EGRESS"] == "1"
	if !hasNetwork {
		args = append(args, "--network", "none")
	} else if !allowEgress {
		args = append(args, "--network", "internal")
	}
	// hasNetwork && allowEgress → default bridge (internet access)

	for k, v := range spec.Env {
		// Skip internal env vars (don't pass to container)
		if k == "MEWORK_NETWORK" || k == "MEWORK_EGRESS" {
			continue
		}
		args = append(args, "-e", k+"="+v)
	}

	// Use sandbox ID for the container name if set.
	if spec.SandboxID != "" {
		args = append(args, "--name", containerName(spec.SandboxID))
	}

	args = append(args, image, "sleep", "infinity")

	var createOut bytes.Buffer
	createCmd := exec.CommandContext(ctx, "docker", args...)
	createCmd.Stdout = &createOut
	createCmd.Stderr = &createOut
	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("docker create: %w\n%s", err, createOut.String())
	}
	return strings.TrimSpace(createOut.String()), nil
}

// Stop stops the container gracefully.
func (d *Driver) Stop(ctx context.Context, sandboxID string) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", containerName(sandboxID))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop: %w\n%s", err, string(out))
	}
	return nil
}

// Destroy removes the container forcibly.
func (d *Driver) Destroy(ctx context.Context, sandboxID string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerName(sandboxID))
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "No such container") {
			return nil
		}
		return fmt.Errorf("docker rm: %w\n%s", err, string(out))
	}
	return nil
}

// ensureImage pulls the image if not present locally.
func ensureImage(ctx context.Context, image string) error {
	checkCmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	if err := checkCmd.Run(); err == nil {
		return nil
	}
	pullCmd := exec.CommandContext(ctx, "docker", "pull", image)
	if out, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker pull: %w\n%s", err, string(out))
	}
	return nil
}

func containerName(id string) string {
	return "mework-" + id
}

// parseDiskLimit parses a disk size string (e.g. "10GiB", "500MiB", "1GiB")
// into bytes. Supports: B, KiB, MiB, GiB, TiB suffixes.
func parseDiskLimit(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty disk limit")
	}

	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "TiB"):
		multiplier = 1 << 40
		s = strings.TrimSuffix(s, "TiB")
	case strings.HasSuffix(s, "GiB"):
		multiplier = 1 << 30
		s = strings.TrimSuffix(s, "GiB")
	case strings.HasSuffix(s, "MiB"):
		multiplier = 1 << 20
		s = strings.TrimSuffix(s, "MiB")
	case strings.HasSuffix(s, "KiB"):
		multiplier = 1 << 10
		s = strings.TrimSuffix(s, "KiB")
	case strings.HasSuffix(s, "B"):
		multiplier = 1
		s = strings.TrimSuffix(s, "B")
	default:
		// Assume bytes if no suffix.
	}

	var val float64
	if _, err := fmt.Sscanf(s, "%f", &val); err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if val <= 0 {
		return 0, fmt.Errorf("disk limit must be positive: %f", val)
	}
	return int64(val * float64(multiplier)), nil
}

