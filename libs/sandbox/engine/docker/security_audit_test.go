// Package docker implements the Docker container sandbox driver.
// Security audit tests here document current isolation gaps before hardening.
package docker

import (
	"bytes"
	"context"
	"os/exec"
	"testing"

	"mework/libs/shared/core"
)

// TestAudit_DockerEngine_CanRunPrivilegedCommands documents gap 0.5: the Docker
// engine runs containers with default capabilities (no --cap-drop), so
// privileged operations like dmesg succeed inside the container.
func TestAudit_DockerEngine_CanRunPrivilegedCommands(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not running")
	}

	drv := New()
	spec := core.RunSpec{
		AgentID:   "audit-priv",
		SandboxID: "audit-priv-" + t.Name(),
	}
	ctx := context.Background()
	sb, err := drv.Start(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer drv.Destroy(ctx, spec.SandboxID)

	var stdout, stderr bytes.Buffer
	exitCode, execErr := sb.Exec(ctx, []string{"dmesg"}, nil, &stdout, &stderr)
	if execErr != nil || exitCode != 0 {
		t.Logf("HARDENED: dmesg blocked (exit=%d)", exitCode)
		return
	}
	t.Logf("GAP CONFIRMED: privileged command dmesg succeeded (no cap-drop)")
}

// TestAudit_DockerEngine_CanWriteToEtc documents that the Docker engine does
// not use --read-only, so the container can write to /etc/.
func TestAudit_DockerEngine_CanWriteToEtc(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not running")
	}

	drv := New()
	spec := core.RunSpec{
		AgentID:   "audit-readonly",
		SandboxID: "audit-readonly-" + t.Name(),
	}
	ctx := context.Background()
	sb, err := drv.Start(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer drv.Destroy(ctx, spec.SandboxID)

	exitCode, execErr := sb.Exec(ctx, []string{"touch", "/etc/mework-audit-test"}, nil, nil, nil)
	if execErr != nil || exitCode != 0 {
		t.Logf("HARDENED: write to /etc/ blocked (exit=%d)", exitCode)
		return
	}
	t.Log("GAP CONFIRMED: container could write to /etc/ (no --read-only)")
}

// TestAudit_DockerEngine_NetworkAccess documents that the Docker engine uses
// the default bridge network, giving containers outbound internet access.
func TestAudit_DockerEngine_NetworkAccess(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not running")
	}

	drv := New()
	spec := core.RunSpec{
		AgentID:   "audit-net",
		SandboxID: "audit-net-" + t.Name(),
	}
	ctx := context.Background()
	sb, err := drv.Start(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer drv.Destroy(ctx, spec.SandboxID)

	exitCode, execErr := sb.Exec(ctx, []string{"sh", "-c",
		"curl -s --connect-timeout 3 https://example.com > /dev/null 2>&1 || true"},
		nil, nil, nil)
	if execErr != nil {
		t.Logf("Network check error: %v", execErr)
	}
	if exitCode != 0 {
		t.Log("HARDENED: outbound network blocked")
		return
	}
	t.Log("GAP CONFIRMED: container has outbound network access (default bridge)")
}

// TestAudit_DockerEngine_UserIsRoot documents that the Docker engine does not
// set --user, so processes run as root inside the container.
func TestAudit_DockerEngine_UserIsRoot(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not running")
	}

	drv := New()
	spec := core.RunSpec{
		AgentID:   "audit-user",
		SandboxID: "audit-user-" + t.Name(),
	}
	ctx := context.Background()
	sb, err := drv.Start(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer drv.Destroy(ctx, spec.SandboxID)

	var uidOut bytes.Buffer
	exitCode, execErr := sb.Exec(ctx, []string{"id", "-u"}, nil, &uidOut, nil)
	if execErr != nil || exitCode != 0 {
		t.Logf("Could not check user: exit=%d err=%v", exitCode, execErr)
		return
	}
	if uidOut.String() != "0\n" {
		t.Logf("HARDENED: container runs as non-root (uid=%s)", uidOut.String())
		return
	}
	t.Log("GAP CONFIRMED: container runs as root (no --user flag)")
}
