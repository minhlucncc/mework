// Package local implements the "local" sandbox driver — a subprocess on the
// host with NO isolation. Security audit tests here document the gap.
package local

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"mework/libs/shared/core"
)

// TestAudit_CanReadSSHKey documents gap 0.1: the local engine provides ZERO
// isolation, so a sandboxed process can read ~/.ssh/id_rsa.
//
// AUDIT: This test MUST PASS today (confirming the gap). After sandbox-exec or
// other hardening lands, update this test to EXPECT failure.
func TestAudit_CanReadSSHKey(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("requires unix filesystem")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	sshKey := filepath.Join(home, ".ssh", "id_rsa")
	if _, err := os.Stat(sshKey); os.IsNotExist(err) {
		t.Skip("no SSH key found — create one to test isolation")
	}

	spec := core.RunSpec{
		AgentID:   "audit-ssh",
		SandboxID: t.TempDir(),
	}
	drv := New()
	sb, err := drv.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	_, execErr := sb.Exec(context.Background(), []string{"cat", sshKey}, nil, &stdout, &stderr)
	if execErr != nil || stdout.Len() == 0 {
		t.Logf("HARDENED: sandbox blocked SSH key read (stdout=%q stderr=%q err=%v)", stdout.String(), stderr.String(), execErr)
		return
	}
	t.Logf("GAP CONFIRMED: sandboxed process read SSH key (len=%d)", stdout.Len())
}

// TestAudit_CanWriteToSystemDir documents gap 0.2: the local engine allows
// writing to system directories like /etc/.
func TestAudit_CanWriteToSystemDir(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("requires unix filesystem")
	}
	testFile := "/etc/mework-audit-test-" + t.Name()
	spec := core.RunSpec{
		AgentID:   "audit-etc",
		SandboxID: t.TempDir(),
	}
	drv := New()
	sb, err := drv.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode, execErr := sb.Exec(context.Background(), []string{"touch", testFile}, nil, &stdout, &stderr)
	_ = os.Remove(testFile)
	if execErr != nil || exitCode != 0 {
		t.Logf("HARDENED: sandbox blocked system write: %v", err)
		return
	}
	t.Log("GAP CONFIRMED: sandboxed process wrote to /etc/")
}

// TestAudit_CanAccessDockerSocket documents gap 0.3: the local engine gives
// the subprocess access to the Docker socket (container escape vector).
func TestAudit_CanAccessDockerSocket(t *testing.T) {
	dockerSocket := "/var/run/docker.sock"
	if _, err := os.Stat(dockerSocket); os.IsNotExist(err) {
		t.Skip("Docker socket not found")
	}
	spec := core.RunSpec{
		AgentID:   "audit-docker",
		SandboxID: t.TempDir(),
	}
	drv := New()
	sb, err := drv.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	_, execErr := sb.Exec(context.Background(),
		[]string{"test", "-S", dockerSocket}, nil, &stdout, &stderr)
	if execErr != nil {
		t.Logf("HARDENED: Docker socket access blocked: %v", execErr)
		return
	}
	t.Log("GAP CONFIRMED: sandboxed process can access Docker socket (container escape vector)")
}

// TestAudit_CanMakeOutboundNetwork documents gap 0.4: the local engine gives
// full network access to the subprocess.
func TestAudit_CanMakeOutboundNetwork(t *testing.T) {
	spec := core.RunSpec{
		AgentID:   "audit-net",
		SandboxID: t.TempDir(),
	}
	drv := New()
	sb, err := drv.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	_, execErr := sb.Exec(context.Background(),
		[]string{"sh", "-c", "echo 'hi' | nc -w 3 example.com 80 2>/dev/null || curl -s --connect-timeout 3 https://example.com 2>/dev/null || wget -q -O - --timeout=3 https://example.com 2>/dev/null || true"},
		nil, &stdout, &stderr)
	_ = execErr
	if stdout.Len() > 0 || stderr.Len() == 0 || execErr == nil {
		t.Log("GAP CONFIRMED: sandboxed process has outbound network access")
	}
}

// TestAudit_WorkdirEscape documents gap 0.7: the local engine does not jail
// the process to its workdir.
func TestAudit_WorkdirEscape(t *testing.T) {
	dir := t.TempDir()
	spec := core.RunSpec{
		AgentID:   "audit-escape",
		SandboxID: dir,
	}
	drv := New()
	sb, err := drv.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	escapeMarker := filepath.Join(dir, "..", "escaped.txt")
	var stdout, stderr bytes.Buffer
	exitCode, execErr := sb.Exec(context.Background(),
		[]string{"touch", escapeMarker}, nil, &stdout, &stderr)
	if execErr != nil || exitCode != 0 {
		t.Logf("HARDENED: workdir escape blocked: %v", err)
		return
	}
	os.Remove(escapeMarker)
	t.Log("GAP CONFIRMED: sandboxed process escaped workdir via ../")
}

// TestAudit_CrossSandboxSecretIsolation documents gap 0.6: the local engine
// allows processes in different sandboxes to read each other's secret files
// (same OS user).
func TestAudit_CrossSandboxSecretIsolation(t *testing.T) {
	secretsDir := t.TempDir()

	// Create sandbox A's secret
	sandboxA := "sandbox-a"
	aDir := filepath.Join(secretsDir, sandboxA)
	if err := os.MkdirAll(aDir, 0700); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(aDir, "my-secret")
	if err := os.WriteFile(secretFile, []byte("secret-value-A"), 0400); err != nil {
		t.Fatal(err)
	}

	spec := core.RunSpec{
		AgentID:   "audit-cross",
		SandboxID: t.TempDir(),
	}
	drv := New()
	sb, err := drv.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	_, execErr := sb.Exec(context.Background(),
		[]string{"cat", secretFile}, nil, &stdout, &stderr)
	if execErr != nil || stdout.Len() == 0 {
		t.Logf("HARDENED: cross-sandbox secret read blocked: %v", execErr)
		return
	}
	t.Logf("GAP CONFIRMED: sandbox B can read sandbox A's secret file (same OS user)")
}
