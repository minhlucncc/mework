package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// executeInit runs initCmd with the given args, capturing stdout, and returns
// the captured output and any RunE error. A context is supplied because RunE
// may use cmd.Context().
func executeInit(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetInitFlags(t)
	var out bytes.Buffer
	initCmd.SetOut(&out)
	initCmd.SetErr(&out)
	initCmd.SetArgs(args)
	initCmd.SetContext(context.Background())
	if err := initCmd.ParseFlags(args); err != nil {
		return out.String(), err
	}
	err := initCmd.RunE(initCmd, args)
	return out.String(), err
}

// resetInitFlags resets initCmd flag state to defaults so each subtest starts
// from a clean slate.
func resetInitFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"workspace", "agent", "name", "backend", "provider"} {
		if f := initCmd.Flags().Lookup(name); f != nil {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
}

// TestInitProviderMezonWritesProviderBlock asserts that
// `mework init --workspace <tmp> --agent claude --name mybot --provider mezon`
// writes a `provider: mezon` block to mework.yml along with a default echo
// policy. This realises the cli delta-spec scenario "mework init --provider
// mezon writes the provider block to mework.yml".
func TestInitProviderMezonWritesProviderBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	dir := t.TempDir()
	out, err := executeInit(t,
		"--workspace", dir,
		"--agent", "claude",
		"--name", "mybot",
		"--provider", "mezon",
	)
	if err != nil {
		t.Fatalf("init: %v\noutput: %s", err, out)
	}

	ymlPath := filepath.Join(dir, "mework.yml")
	body, readErr := os.ReadFile(ymlPath)
	if readErr != nil {
		t.Fatalf("read mework.yml: %v", readErr)
	}
	yml := string(body)
	if !strings.Contains(yml, "provider: mezon") {
		t.Errorf("mework.yml missing `provider: mezon` line:\n%s", yml)
	}
	// A default policy block must also be written. The exact key name is
	// part of the GREEN contract; assert the policy: header is present.
	if !strings.Contains(yml, "policy:") {
		t.Errorf("mework.yml missing `policy:` block:\n%s", yml)
	}
}

// TestInitProviderUnknownIsRejected asserts that `--provider slack` (or any
// value other than the allowed set) is rejected with a usage error mentioning
// "unsupported provider". This realises the cli delta-spec scenario for
// `--provider` validation.
func TestInitProviderUnknownIsRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	dir := t.TempDir()
	out, err := executeInit(t,
		"--workspace", dir,
		"--agent", "claude",
		"--name", "mybot",
		"--provider", "slack",
	)
	if err == nil {
		t.Fatalf("expected error for unsupported provider, got nil\noutput: %s", out)
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("error %q must mention `unsupported provider`", err.Error())
	}

	// mework.yml must NOT have been written on the error path.
	ymlPath := filepath.Join(dir, "mework.yml")
	if _, statErr := os.Stat(ymlPath); statErr == nil {
		t.Errorf("mework.yml must not be written when --provider is rejected, but it exists")
	}
}

// TestInitNoProviderKeepsOldBehavior asserts that `mework init` without
// `--provider` does NOT add a `provider:` key to mework.yml. This realises
// the cli delta-spec scenario "mework init without --provider keeps old
// behavior".
func TestInitNoProviderKeepsOldBehavior(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	dir := t.TempDir()
	out, err := executeInit(t,
		"--workspace", dir,
		"--agent", "claude",
		"--name", "mybot",
	)
	if err != nil {
		t.Fatalf("init: %v\noutput: %s", err, out)
	}

	ymlPath := filepath.Join(dir, "mework.yml")
	body, readErr := os.ReadFile(ymlPath)
	if readErr != nil {
		t.Fatalf("read mework.yml: %v", readErr)
	}
	if strings.Contains(string(body), "provider:") {
		t.Errorf("mework.yml must NOT contain `provider:` line without --provider, got:\n%s", string(body))
	}
}