package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// executeProviderMezonSet runs `mework provider mezon set --app-id ...
// --api-key ... --base-url ...` via cobra. Flags are parsed from args and
// RunE is then invoked.
func executeProviderMezonSet(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	providerMezonSetCmd.SetOut(&out)
	providerMezonSetCmd.SetErr(&out)
	providerMezonSetCmd.SetArgs(args)
	providerMezonSetCmd.SetContext(context.Background())
	if err := providerMezonSetCmd.ParseFlags(args); err != nil {
		return out.String(), err
	}
	err := providerMezonSetCmd.RunE(providerMezonSetCmd, args)
	return out.String(), err
}

// executeProviderMezonShow runs `mework provider mezon show` via cobra.
func executeProviderMezonShow(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	providerMezonShowCmd.SetOut(&out)
	providerMezonShowCmd.SetErr(&out)
	providerMezonShowCmd.SetArgs(args)
	providerMezonShowCmd.SetContext(context.Background())
	if err := providerMezonShowCmd.ParseFlags(args); err != nil {
		return out.String(), err
	}
	err := providerMezonShowCmd.RunE(providerMezonShowCmd, args)
	return out.String(), err
}

// TestProviderMezonSetWritesCredentialsAt0600 asserts that `mework provider
// mezon set` persists credentials at <tmpHome>/.mework/provider/mezon/
// credentials.json with mode 0600. This realises the cli delta-spec
// requirement that the offline orchestrator can safely read the credential
// file at the documented path.
func TestProviderMezonSetWritesCredentialsAt0600(t *testing.T) {
	// Point MEWORK_HOME at a temp dir; the credential file lives under
	// <MEWORK_HOME>/provider/mezon/credentials.json (the same path the
	// offline-stack orchestrator reads).
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	credDir := filepath.Join(home, "provider", "mezon")
	credPath := filepath.Join(credDir, "credentials.json")

	out, err := executeProviderMezonSet(t,
		"--app-id", "app-xyz",
		"--api-key", "secret-abc",
		"--base-url", "https://api.mezon.vn",
	)
	if err != nil {
		t.Fatalf("provider mezon set: %v\noutput: %s", err, out)
	}

	fi, statErr := os.Stat(credPath)
	if statErr != nil {
		t.Fatalf("credentials file not written at %s: %v\noutput: %s", credPath, statErr, out)
	}
	got := fi.Mode().Perm()
	if got != 0o600 {
		t.Errorf("credentials file mode = %#o, want %#o", got, 0o600)
	}
}

// TestProviderMezonShowReadsCredentialsAt0600 asserts that `mework provider
// mezon show` only loads the credentials file when its mode is exactly 0600;
// otherwise it refuses + warns. We write the file at 0644 and expect the
// command to fail closed.
func TestProviderMezonShowReadsCredentialsAt0600(t *testing.T) {
	// Set MEWORK_HOME to a temp dir; lay down an insecure (0644) credentials
	// file there and assert the show command refuses to read it.
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	credDir := filepath.Join(home, "provider", "mezon")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credPath := filepath.Join(credDir, "credentials.json")
	body := `{"app_id":"app-xyz","api_key":"secret-abc","base_url":"https://api.mezon.vn"}`
	if err := os.WriteFile(credPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := executeProviderMezonShow(t)
	if err == nil {
		t.Fatalf("expected refusal for insecure perms; got nil error\noutput: %s", out)
	}
	// The error message must make the cause clear so the user can fix it.
	msg := strings.ToLower(err.Error())
	if !(strings.Contains(msg, "insecure") || strings.Contains(msg, "0600") || strings.Contains(msg, "permission")) {
		t.Errorf("error %q must explain the perm refusal (insecure / 0600 / permission)", err.Error())
	}

	// Tightening to 0600 must allow the show command to succeed.
	if err := os.Chmod(credPath, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = executeProviderMezonShow(t)
	if err != nil {
		t.Errorf("expected success after chmod 0600; got %v\noutput: %s", err, out)
	}
}