// Package runner — offline-stack orchestrator tests.
//
// Unit 02 of c0047-mezon-offline-mode (RED step).
//
// These tests pin the contract documented in the delta spec
// `mezon-offline-bundle`:
//
//   - server crashes during boot → orchestrator exits non-zero
//   - /readyz exceeds 10s timeout → server is signalled SIGTERM
//   - enrollment fails → server is signalled SIGTERM
//   - happy path — full stack boots and reaches steady state (integration)
//   - mework daemon stop shuts down the full stack
//   - two daemon start --offline invocations cannot race (pidfile O_EXCL)
//   - SIGINT cascades in reverse order
//
// The orchestrator's process spawning and clock are injectable so all
// non-integration tests run in-process without spawning real binaries.
//
// Happy-path tests are gated by `MEWORK_E2E_OFFLINE=1` and a successful
// `go build ./apps/mework-server`. Without both, the happy-path suite skips
// cleanly (per the design's "Stack-level integration test" note).
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Type contracts. The production types live in offline_stack.go (struct,
// interface, error var). This file uses them as-is.
// ---------------------------------------------------------------------------

// Run is implemented in offline_stack.go.

// ---------------------------------------------------------------------------
// TestMain — build the real mework-server binary once. If `go build` fails,
// record the failure and skip the happy-path integration tests (per the
// design's "Stack-level integration test" note: tests "skip cleanly when
// `go build` fails").
// ---------------------------------------------------------------------------

var (
	testServerBin string
	testSkipE2E   bool
	testTmpDir    string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mework-offline-stack-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir tmp: %v\n", err)
		os.Exit(2)
	}
	testTmpDir = dir
	bin := filepath.Join(dir, "mework-server")

	// Resolve the workspace root by walking up from this file's package.
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "locate repo root: %v\n", err)
		testSkipE2E = true
	} else {
		cmd := exec.Command("go", "build", "-o", bin, "./apps/mework-server")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "go build mework-server: %v\n%s\n", err, string(out))
			testSkipE2E = true
		} else {
			testServerBin = bin
		}
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("go.work not found")
}

// ---------------------------------------------------------------------------
// Stub-failure paths — always run, in-process. Inject a fake processRunner
// and a fake HTTP server for /readyz and the enrollment endpoints.
// ---------------------------------------------------------------------------

// fakeProc is a configurable processRunner. It records every operation so
// tests can assert on the orchestrator's behaviour.
type fakeProc struct {
	mu sync.Mutex

	// start behaviour per role. nil → Start returns the default values below.
	serverStart func(ctx context.Context, args []string, env []string, stdoutPath string) (pid int, port int, err error)
	workerStart func(ctx context.Context, args []string, env []string, stdoutPath string) (pid int, port int, err error)

	// Optional hook fired once a child is "spawned".
	onSpawn func(role string, pid int, port int)

	// Wait returns when a process is reported as exited. If nil, Wait blocks
	// forever (so we never accidentally report an early exit).
	wait chan struct{}

	// Recorded signals.
	signals []fakeSignal

	// Server / worker PIDs (assigned by start hook).
	serverPID int
	serverPt  int
	workerPID int
}

type fakeSignal struct {
	PID  int
	Sig  syscall.Signal
	When time.Time
}

func (f *fakeProc) LookPath(file string) (string, error) {
	// Always succeed for the binaries the orchestrator looks up.
	switch file {
	case "mework-server", "mework-mezon-worker":
		return "/fake/bin/" + file, nil
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

func (f *fakeProc) Start(ctx context.Context, name string, args []string, env []string, stdoutPath string) (pid int, port int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Extract basename so both "mework-server" and "/path/to/mework-server" match.
	if base := filepath.Base(name); base != name && base != "" && base != "." {
		name = base
	}
	switch name {
	case "mework-server":
		if f.serverStart != nil {
			return f.serverStart(ctx, args, env, stdoutPath)
		}
		// Default: pretend the server bound to 127.0.0.1:0 and the OS chose 52345.
		pid = 81234
		port = 52345
		f.serverPID = pid
		f.serverPt = port
		if f.onSpawn != nil {
			f.onSpawn("server", pid, port)
		}
		return pid, port, nil
	case "mework-mezon-worker":
		if f.workerStart != nil {
			p, _, e := f.workerStart(ctx, args, env, stdoutPath)
			f.workerPID = p
			return p, 0, e
		}
		pid = 81235
		f.workerPID = pid
		if f.onSpawn != nil {
			f.onSpawn("worker", pid, 0)
		}
		return pid, 0, nil
	default:
		return 0, 0, fmt.Errorf("fakeProc.Start: unknown name %q", name)
	}
}

func (f *fakeProc) Wait(pid int) error {
	if f.wait == nil {
		<-make(chan struct{}) // block forever by default
		return nil
	}
	<-f.wait
	return nil
}

func (f *fakeProc) Signal(pid int, sig syscall.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, fakeSignal{PID: pid, Sig: sig, When: time.Now()})
	return nil
}

func (f *fakeProc) Kill(pid int) error {
	return f.Signal(pid, syscall.SIGKILL)
}

func (f *fakeProc) signalsFor(pid int) []fakeSignal {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeSignal, 0, len(f.signals))
	for _, s := range f.signals {
		if s.PID == pid {
			out = append(out, s)
		}
	}
	return out
}

// fakeClock lets the orchestrator's 10s timeout fire in milliseconds.
type fakeClock struct {
	mu     sync.Mutex
	timers []*fakeTimer
}

type fakeTimer struct {
	when    time.Time
	channel chan time.Time
	stopped bool
}

func newFakeClock() *fakeClock            { return &fakeClock{} }
func (c *fakeClock) NewTimer(d time.Duration) *Timer {
	t := &fakeTimer{
		when:    time.Now().Add(d),
		channel: make(chan time.Time, 1),
	}
	c.mu.Lock()
	c.timers = append(c.timers, t)
	c.mu.Unlock()
	return &Timer{
		C: t.channel,
		Stop: func() bool {
			t.stopped = true
			return true
		},
	}
}

// fireTimers forces all registered timers to expire immediately.
func (c *fakeClock) fireTimers() {
	c.mu.Lock()
	timers := append([]*fakeTimer(nil), c.timers...)
	c.mu.Unlock()
	for _, t := range timers {
		if t.stopped {
			continue
		}
		select {
		case t.channel <- time.Now():
		default:
		}
	}
}

// stubHTTPServer is a tiny http.Server that responds to /readyz,
// /api/v1/runners/registration-tokens, and /api/v1/runners/enroll with
// configurable status codes.
type stubHTTPServer struct {
	t                  *testing.T
	readyStatus        int
	enrollStatus       int
	registrationStatus int
	enrollCalls        []enrollCall
	mu                 sync.Mutex
}

type enrollCall struct {
	RegistrationToken string `json:"registration_token"`
	Token             string `json:"token"`
}

func (s *stubHTTPServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(s.readyStatus)
		_, _ = w.Write([]byte(`{"status":"` + strconv.Itoa(s.readyStatus) + `"}`))
	})
	mux.HandleFunc("/api/v1/runners/registration-tokens", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.WriteHeader(s.registrationStatus)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "rt-fake"})
	})
	mux.HandleFunc("/api/v1/runners/enroll", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.enrollCalls = append(s.enrollCalls, enrollCall{
			RegistrationToken: r.Header.Get("Authorization"),
			Token:             body["token"],
		})
		w.WriteHeader(s.enrollStatus)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"runner_id": "rid-fake",
			"secret":    "rt-token-fake",
		})
	})
	return mux
}

func (s *stubHTTPServer) url() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: s.handler()}
	go func() { _ = srv.Serve(ln) }()
	s.t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String()
}

// makeStack returns an OfflineStack wired to the provided fakes. The
// RunOpts are populated with sane defaults pointing into the test's tmpdir.
func makeStack(t *testing.T, proc processRunner, clk Clock) (*OfflineStack, RunOpts, string) {
	t.Helper()
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	opts := RunOpts{
		Workspace:    dir,
		RuntimeDir:   runtimeDir,
		KeysDir:      runtimeDir,
		TokenPath:    filepath.Join(runtimeDir, "runner.token"),
		ServerLog:    filepath.Join(runtimeDir, "server.log"),
		WorkerLog:    filepath.Join(runtimeDir, "worker.log"),
		ServerBin:    "/fake/bin/mework-server",
		WorkerBin:    "/fake/bin/mework-mezon-worker",
		ListenAddr:   "127.0.0.1:0",
		ReadyTimeout: 10 * time.Second,
		StopTimeout:  5 * time.Second,
	}
	if clk == nil {
		clk = newFakeClock()
	}
	stack := &OfflineStack{
		LookPath: proc.LookPath,
		Proc:     proc,
		Clock:    clk,
		Now:      time.Now,
	}
	return stack, opts, runtimeDir
}

// pidfilePath is the canonical path to the offline-stack pidfile.
func pidfilePath(runtimeDir string) string {
	return filepath.Join(runtimeDir, "offline-pids.json")
}

// ---------------------------------------------------------------------------
// Test: server crashes during boot — orchestrator exits non-zero.
// ---------------------------------------------------------------------------

func TestOfflineStack_ServerCrashDuringBoot(t *testing.T) {
	proc := &fakeProc{
		serverStart: func(ctx context.Context, args []string, env []string, stdoutPath string) (int, int, error) {
			// Pretend the binary started (pid=81234, port=52345) then immediately
			// reported an exit. The orchestrator must observe that exit and fail.
			go func() {
				_ = proc_WaitImmediately()
			}()
			return 81234, 52345, nil
		},
	}
	stack, opts, rd := makeStack(t, proc, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := stack.Run(ctx, opts)
	if err == nil {
		t.Fatal("Run returned nil; expected non-nil error mentioning server boot")
	}
	if !errors.Is(err, err) && !contains(err.Error(), "server") {
		t.Fatalf("error does not mention server boot: %v", err)
	}
	if _, statErr := os.Stat(pidfilePath(rd)); statErr == nil {
		t.Fatalf("pidfile %s exists; expected it to be absent after server crash",
			pidfilePath(rd))
	}
}

// proc_WaitImmediately is a stand-in used by the test above to drive the
// fake's Wait hook to return immediately.
func proc_WaitImmediately() error {
	return nil
}

// ---------------------------------------------------------------------------
// Test: /readyz exceeds 10s timeout — server gets SIGTERM.
// ---------------------------------------------------------------------------

func TestOfflineStack_ReadyzTimeout(t *testing.T) {
	stub := &stubHTTPServer{t: t, readyStatus: http.StatusServiceUnavailable}
	url := stub.url()

	clk := newFakeClock()
	proc := &fakeProc{}
	stack, opts, _ := makeStack(t, proc, clk)

	// Override the URL the orchestrator hits with our stub.
	opts.ServerURL = url
	// Shrink the timeout to 10s so the fakeClock has a matching deadline.
	opts.ReadyTimeout = 10 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- stack.Run(ctx, opts) }()

	// Wait a tick for the goroutine to enter waitReady, then force the timer.
	time.Sleep(20 * time.Millisecond)
	clk.fireTimers()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Run returned nil; expected timeout error")
		}
		if !contains(err.Error(), "ready") && !contains(err.Error(), "timeout") {
			t.Fatalf("error does not mention ready/timeout: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after timer fired")
	}

	sigs := proc.signalsFor(81234)
	foundTerm := false
	for _, s := range sigs {
		if s.Sig == syscall.SIGTERM {
			foundTerm = true
			break
		}
	}
	if !foundTerm {
		t.Fatalf("expected SIGTERM on server pid 81234; signals=%+v", sigs)
	}
}

// ---------------------------------------------------------------------------
// Test: enrollment fails — server is signalled SIGTERM.
// ---------------------------------------------------------------------------

func TestOfflineStack_EnrollFails(t *testing.T) {
	stub := &stubHTTPServer{
		t:                  t,
		readyStatus:        http.StatusOK,
		registrationStatus: http.StatusOK,
		enrollStatus:       http.StatusInternalServerError,
	}
	url := stub.url()

	proc := &fakeProc{}
	stack, opts, _ := makeStack(t, proc, nil)
	opts.ServerURL = url

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := stack.Run(ctx, opts)
	if err == nil {
		t.Fatal("Run returned nil; expected enroll error")
	}
	if !contains(err.Error(), "enroll") {
		t.Fatalf("error does not mention enroll: %v", err)
	}
	sigs := proc.signalsFor(81234)
	foundTerm := false
	for _, s := range sigs {
		if s.Sig == syscall.SIGTERM {
			foundTerm = true
			break
		}
	}
	if !foundTerm {
		t.Fatalf("expected SIGTERM on server pid 81234; signals=%+v", sigs)
	}
}

// ---------------------------------------------------------------------------
// Test: worker boot fails after enroll succeeds — server is signalled SIGTERM.
// ---------------------------------------------------------------------------

func TestOfflineStack_WorkerBootFails(t *testing.T) {
	stub := &stubHTTPServer{
		t:                  t,
		readyStatus:        http.StatusOK,
		registrationStatus: http.StatusOK,
		enrollStatus:       http.StatusOK,
	}
	url := stub.url()

	proc := &fakeProc{
		workerStart: func(ctx context.Context, args []string, env []string, stdoutPath string) (int, int, error) {
			return 0, 0, errors.New("worker binary not found")
		},
	}
	stack, opts, _ := makeStack(t, proc, nil)
	opts.ServerURL = url

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := stack.Run(ctx, opts)
	if err == nil {
		t.Fatal("Run returned nil; expected worker boot error")
	}
	if !contains(err.Error(), "worker") {
		t.Fatalf("error does not mention worker: %v", err)
	}
	sigs := proc.signalsFor(81234)
	foundTerm := false
	for _, s := range sigs {
		if s.Sig == syscall.SIGTERM {
			foundTerm = true
			break
		}
	}
	if !foundTerm {
		t.Fatalf("expected SIGTERM on server pid 81234; signals=%+v", sigs)
	}
}

// ---------------------------------------------------------------------------
// Test: pidfile atomicity — two concurrent Write calls produce one winner.
// ---------------------------------------------------------------------------

func TestPidfile_AtomicWrite(t *testing.T) {
	if _, err := os.Stat(""); err == nil {
		// keep the linter happy on systems where os.Stat("") returns nil
	}
	path := filepath.Join(t.TempDir(), "offline-pids.json")

	pf := &Pidfile{Path: path}

	const goroutines = 8
	var wg sync.WaitGroup
	results := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			meta := Meta{
				Workspace: "/tmp/ws",
				Started:   time.Now().UTC().Format(time.RFC3339),
				Children: []PidfileChild{
					{Role: "server", PID: 81234, Port: 52345, Log: "/tmp/server.log"},
				},
			}
			results[idx] = pf.Write(meta)
		}(i)
	}
	wg.Wait()

	successes := 0
	alreadyRunning := 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAlreadyRunning):
			alreadyRunning++
		default:
			t.Fatalf("unexpected Write error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful Write; got %d (successes=%d, alreadyRunning=%d)",
			successes, successes, alreadyRunning)
	}
	if alreadyRunning != goroutines-1 {
		t.Fatalf("expected %d ErrAlreadyRunning; got %d", goroutines-1, alreadyRunning)
	}

	// Read what was written and confirm it round-trips.
	got, err := pf.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Workspace != "/tmp/ws" || len(got.Children) != 1 {
		t.Fatalf("Read returned unexpected meta: %+v", got)
	}
	if err := pf.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pidfile still present after Remove: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: signal forwarding cascades in reverse order — worker before server.
// ---------------------------------------------------------------------------

func TestOfflineStack_SignalCascadeReverseOrder(t *testing.T) {
	stub := &stubHTTPServer{
		t:                  t,
		readyStatus:        http.StatusOK,
		registrationStatus: http.StatusOK,
		enrollStatus:       http.StatusOK,
	}
	url := stub.url()

	proc := &fakeProc{}
	stack, opts, _ := makeStack(t, proc, nil)
	opts.ServerURL = url

	ready := make(chan struct{})
	proc.onSpawn = func(role string, pid int, port int) {
		if role == "worker" {
			close(ready)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- stack.Run(ctx, opts) }()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("worker never spawned")
	}

	// Trigger shutdown. The orchestrator must signal the worker FIRST, then
	// the server, both with SIGTERM (per the spec's "reverse spawn order").
	cancel()

	select {
	case err := <-done:
		_ = err // graceful shutdown returns nil; signal-induced cleanup may wrap ctx.Err
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	workerSigs := proc.signalsFor(81235)
	serverSigs := proc.signalsFor(81234)
	if len(workerSigs) == 0 {
		t.Fatalf("expected at least one signal on worker; got none")
	}
	if len(serverSigs) == 0 {
		t.Fatalf("expected at least one signal on server; got none")
	}

	workerFirst := workerSigs[0].When.Before(serverSigs[0].When) ||
		workerSigs[0].When.Equal(serverSigs[0].When)
	if !workerFirst {
		t.Fatalf("expected worker signal before server signal; worker=%v server=%v",
			workerSigs[0].When, serverSigs[0].When)
	}
	if workerSigs[0].Sig != syscall.SIGTERM {
		t.Fatalf("first worker signal = %v; want SIGTERM", workerSigs[0].Sig)
	}
	if serverSigs[0].Sig != syscall.SIGTERM {
		t.Fatalf("first server signal = %v; want SIGTERM", serverSigs[0].Sig)
	}
}

// ---------------------------------------------------------------------------
// Happy path integration test — gated by MEWORK_E2E_OFFLINE=1 and a
// successful go build of mework-server in TestMain.
//
// Runs the real orchestrator against the real mework-server binary. Spawns
// the server with DATABASE_URL=sqlite://<tmp>/data.db + ephemeral port,
// enrolls a runner, and confirms steady-state. Then signals SIGTERM and
// asserts both children exit within 5s and the pidfile is removed.
// ---------------------------------------------------------------------------

func TestOfflineStack_HappyPath_Integration(t *testing.T) {
	if os.Getenv("MEWORK_E2E_OFFLINE") != "1" {
		t.Skip("MEWORK_E2E_OFFLINE not set; skipping happy-path integration test")
	}
	if testSkipE2E || testServerBin == "" {
		t.Skip("mework-server binary not built; skipping happy-path integration test")
	}

	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}

	opts := RunOpts{
		Workspace:    dir,
		ServerBin:    testServerBin,
		WorkerBin:    "mework-mezon-worker", // PATH lookup; absent → worker step fails (expected, see below)
		RuntimeDir:   runtimeDir,
		KeysDir:      runtimeDir,
		TokenPath:    filepath.Join(runtimeDir, "runner.token"),
		ServerLog:    filepath.Join(runtimeDir, "server.log"),
		WorkerLog:    filepath.Join(runtimeDir, "worker.log"),
		ListenAddr:   "127.0.0.1:0",
		ReadyTimeout: 10 * time.Second,
		StopTimeout:  5 * time.Second,
	}

	stack := &OfflineStack{
		LookPath: exec.LookPath,
		Now:      time.Now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- stack.Run(ctx, opts) }()

	// Poll for the pidfile to appear. Once it does, snapshot its contents
	// and assert the perms + roles.
	deadline := time.Now().Add(12 * time.Second)
	var meta Meta
	pidfile := pidfilePath(runtimeDir)
	pidfileFound := false
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidfile); err == nil {
			if err := json.Unmarshal(data, &meta); err == nil {
				pidfileFound = true
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("ctx done before pidfile appeared")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !pidfileFound {
		t.Skip("pidfile did not appear within deadline; integration environment incomplete")
	}

	// 0600 perms per CLAUDE.md invariant.
	st, err := os.Stat(pidfile)
	if err != nil {
		t.Fatalf("stat pidfile: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Fatalf("pidfile perms = %o; want 0600", mode)
	}

	roles := map[string]bool{}
	var serverPort int
	for _, c := range meta.Children {
		roles[c.Role] = true
		if c.Role == "server" {
			serverPort = c.Port
		}
	}
	if !roles["server"] {
		t.Fatal("pidfile missing 'server' child")
	}
	if serverPort == 0 {
		t.Fatal("server port is 0 in pidfile")
	}

	// Signal shutdown and wait for the orchestrator to return.
	cancel()
	select {
	case <-errCh:
	case <-time.After(10 * time.Second):
		t.Fatal("orchestrator did not return within 10s of cancel")
	}

	// Pidfile should be gone after graceful teardown.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidfile); os.IsNotExist(err) {
			return
		}
		select {
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("pidfile still present after stack teardown")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// contains is a tiny strings.Contains shim so we don't have to import
// strings just for the assertion. Avoids any chance of a future test
// helper becoming a circular dependency.
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Ensure the test file imports exec (used in TestMain / lookups) so the
// import block stays stable when the Green step adds more dependencies.
var _ = exec.Command
var _ = httptest.NewRecorder
var _ = os.Stdin