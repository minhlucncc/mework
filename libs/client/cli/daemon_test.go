package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mework/libs/client/runner"
)

// runOfflineMezonStackCall records the args captured by the test-injected
// runOfflineMezonStack fake so tests can assert what the daemon start command
// would have called.
type runOfflineMezonStackCall struct {
	opts runner.RunOpts
}

// runOfflineMezonStackRecord captures the last call when overridden in tests.
var runOfflineMezonStackRecord atomic.Pointer[runOfflineMezonStackCall]

// withRunOfflineMezonStackFake installs a recording fake for
// runOfflineMezonStack (declared in daemon.go) and returns a cleanup function
// the caller must defer.
func withRunOfflineMezonStackFake(t *testing.T) func() {
	t.Helper()
	prev := runOfflineMezonStack
	runOfflineMezonStackRecord.Store(nil)
	runOfflineMezonStack = func(ctx context.Context, opts runner.RunOpts) error {
		runOfflineMezonStackRecord.Store(&runOfflineMezonStackCall{opts: opts})
		return nil
	}
	return func() {
		runOfflineMezonStack = prev
		runOfflineMezonStackRecord.Store(nil)
	}
}

// lastMezonCall returns the most recently captured call record (nil if none).
func lastMezonCall() *runOfflineMezonStackCall {
	return runOfflineMezonStackRecord.Load()
}

// resetDaemonStartFlags resets daemonStartCmd flag state to defaults so each
// subtest starts from a clean slate. Without this, repeated runs leak flag
// values across cases.
func resetDaemonStartFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"offline", "foreground", "workspace", "with-mezon", "no-server"} {
		if f := daemonStartCmd.Flags().Lookup(name); f != nil {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
}

// runDaemonStartWithAutoCancel runs RunE in a goroutine with a context that
// auto-cancels after `cancelAfter`. This prevents the blocking offline flow
// (which waits on ctx.Done) from hanging the test forever. Returns the
// captured output and the RunE error (nil if the goroutine did not return in
// time and we cancelled it).
func runDaemonStartWithAutoCancel(t *testing.T, cancelAfter time.Duration) (string, error) {
	t.Helper()
	var out bytes.Buffer
	daemonStartCmd.SetOut(&out)
	daemonStartCmd.SetErr(&out)

	ctx, cancel := context.WithCancel(context.Background())
	daemonStartCmd.SetContext(ctx)

	done := make(chan error, 1)
	go func() {
		done <- daemonStartCmd.RunE(daemonStartCmd, []string{})
	}()
	select {
	case err := <-done:
		cancel()
		return out.String(), err
	case <-time.After(cancelAfter):
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		return out.String(), nil
	}
}

// TestDaemonStartHelpAdvertisesMezonFlags asserts that `daemon start --help`
// (or its UsageString) advertises the new flags `--with-mezon` and
// `--no-server` introduced by this change. This realises the cli delta-spec
// scenario "New flags are part of the documented command surface".
func TestDaemonStartHelpAdvertisesMezonFlags(t *testing.T) {
	resetDaemonStartFlags(t)
	out := daemonStartCmd.UsageString()
	if !strings.Contains(out, "--with-mezon") {
		t.Errorf("daemon start help missing --with-mezon flag\n--- output ---\n%s\n--- end ---", out)
	}
	if !strings.Contains(out, "--no-server") {
		t.Errorf("daemon start help missing --no-server flag\n--- output ---\n%s\n--- end ---", out)
	}
}

// TestDaemonStartOfflineWithMezonDelegates asserts that `daemon start --offline
// --with-mezon --workspace <dir>` delegates to the offline-stack orchestrator
// with the expected RunOpts. This realises the daemon-runtime delta-spec
// scenario "--offline --with-mezon boots an offline stack".
func TestDaemonStartOfflineWithMezonDelegates(t *testing.T) {
	cleanup := withRunOfflineMezonStackFake(t)
	defer cleanup()
	resetDaemonStartFlags(t)

	dir := t.TempDir()
	_ = daemonStartCmd.Flags().Set("offline", "true")
	_ = daemonStartCmd.Flags().Set("with-mezon", "true")
	_ = daemonStartCmd.Flags().Set("workspace", dir)

	// Tolerate any nil-func panic while the GREEN wiring is absent.
	func() {
		defer func() { _ = recover() }()
		_, _ = runDaemonStartWithAutoCancel(t, 2*time.Second)
	}()

	called := lastMezonCall()
	if called == nil {
		t.Fatalf("expected runOfflineMezonStack to be invoked with --offline --with-mezon; it was not. Indicates the wiring is not yet present.")
	}
	if got := called.opts.Workspace; got != dir {
		t.Errorf("RunOpts.Workspace = %q, want %q", got, dir)
	}
}

// TestDaemonStartOfflineKeepsPureCLIBehavior asserts that `daemon start
// --offline --workspace <dir>` (without --with-mezon) does NOT delegate to
// the offline-stack orchestrator. This realises the daemon-runtime delta-spec
// scenario "--offline without --with-mezon keeps pure-CLI behavior".
func TestDaemonStartOfflineKeepsPureCLIBehavior(t *testing.T) {
	cleanup := withRunOfflineMezonStackFake(t)
	defer cleanup()
	resetDaemonStartFlags(t)

	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	// Create a valid workspace mework.yml so the existing pure-CLI flow can
	// pass validation when (and only when) the GREEN code routes us there.
	dir := t.TempDir()
	ymlPath := dir + "/mework.yml"
	if err := os.WriteFile(ymlPath, []byte("engine: local\nbackend: echo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_ = daemonStartCmd.Flags().Set("offline", "true")
	_ = daemonStartCmd.Flags().Set("workspace", dir)

	func() {
		defer func() { _ = recover() }()
		_, _ = runDaemonStartWithAutoCancel(t, 2*time.Second)
	}()

	if called := lastMezonCall(); called != nil {
		t.Errorf("runOfflineMezonStack was called for pure-CLI offline flow (Workspace=%q); it must NOT be called without --with-mezon", called.opts.Workspace)
	}
}

// TestDaemonStartOfflineNoServerNoMezonIsNoop asserts that `--offline
// --no-server --workspace <dir>` (no --with-mezon) does NOT delegate to the
// offline-stack orchestrator. Per the design, `--no-server` is a no-op when
// --with-mezon is absent (existing pure-CLI flow runs unchanged).
func TestDaemonStartOfflineNoServerNoMezonIsNoop(t *testing.T) {
	cleanup := withRunOfflineMezonStackFake(t)
	defer cleanup()
	resetDaemonStartFlags(t)

	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	dir := t.TempDir()
	ymlPath := dir + "/mework.yml"
	if err := os.WriteFile(ymlPath, []byte("engine: local\nbackend: echo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_ = daemonStartCmd.Flags().Set("offline", "true")
	_ = daemonStartCmd.Flags().Set("no-server", "true")
	_ = daemonStartCmd.Flags().Set("workspace", dir)

	func() {
		defer func() { _ = recover() }()
		_, _ = runDaemonStartWithAutoCancel(t, 2*time.Second)
	}()

	if called := lastMezonCall(); called != nil {
		t.Errorf("runOfflineMezonStack was called with --no-server but no --with-mezon (Workspace=%q); it must not be called", called.opts.Workspace)
	}
}

// TestDaemonStartWithMezonWithoutOfflineIsRejected asserts that `--with-mezon`
// without `--offline` is rejected and the orchestrator fake is not called.
func TestDaemonStartWithMezonWithoutOfflineIsRejected(t *testing.T) {
	cleanup := withRunOfflineMezonStackFake(t)
	defer cleanup()
	resetDaemonStartFlags(t)

	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	dir := t.TempDir()
	_ = daemonStartCmd.Flags().Set("with-mezon", "true")
	_ = daemonStartCmd.Flags().Set("workspace", dir)

	var out bytes.Buffer
	daemonStartCmd.SetOut(&out)
	daemonStartCmd.SetErr(&out)

	var runErr error
	func() {
		defer func() { _ = recover() }()
		runErr = daemonStartCmd.RunE(daemonStartCmd, []string{})
	}()

	if called := lastMezonCall(); called != nil {
		t.Errorf("runOfflineMezonStack must not be called without --offline; was called with Workspace=%q", called.opts.Workspace)
	}
	if runErr == nil {
		t.Errorf("expected an error rejecting --with-mezon without --offline; got nil. Output:\n%s", out.String())
	} else if !strings.Contains(runErr.Error(), "--with-mezon") {
		t.Errorf("error %q must mention --with-mezon to be actionable", runErr.Error())
	}
}

// TestDaemonStartNoServerWithoutOfflineIsRejected asserts that `--no-server`
// without `--offline` is rejected and the orchestrator fake is not called.
func TestDaemonStartNoServerWithoutOfflineIsRejected(t *testing.T) {
	cleanup := withRunOfflineMezonStackFake(t)
	defer cleanup()
	resetDaemonStartFlags(t)

	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	dir := t.TempDir()
	_ = daemonStartCmd.Flags().Set("no-server", "true")
	_ = daemonStartCmd.Flags().Set("workspace", dir)

	var out bytes.Buffer
	daemonStartCmd.SetOut(&out)
	daemonStartCmd.SetErr(&out)

	var runErr error
	func() {
		defer func() { _ = recover() }()
		runErr = daemonStartCmd.RunE(daemonStartCmd, []string{})
	}()

	if called := lastMezonCall(); called != nil {
		t.Errorf("runOfflineMezonStack must not be called without --offline; was called with Workspace=%q", called.opts.Workspace)
	}
	if runErr == nil {
		t.Errorf("expected an error rejecting --no-server without --offline; got nil. Output:\n%s", out.String())
	} else if !strings.Contains(runErr.Error(), "--no-server") {
		t.Errorf("error %q must mention --no-server to be actionable", runErr.Error())
	}
}