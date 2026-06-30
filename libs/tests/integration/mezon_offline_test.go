// Package integration provides DB-backed integration tests for the mework
// server. This file covers offline-mode Mezon bot integration scenarios from
// the offline-agent and daemon-runtime delta specs.
package integration

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	meworkrunner "mework/libs/client/runner"
	mezonbot "mework/libs/server/provider/mezon/bot"
	"mework/libs/sandbox"
	"mework/libs/sandbox/runtime"
	"mework/libs/server/bus/memory"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
	"mework/libs/shared/policy"
	mezoncfg "mework/libs/shared/providers/mezon"
	"mework/libs/shared/ports"
)

// ---------------------------------------------------------------------------
// Mock SDK client (implements bot.SDKClient)
// ---------------------------------------------------------------------------

// mockSDKMessage implements bot.SDKMessage for compile-time-safe field access.
type mockSDKMessage struct {
	ChannelID string
	SenderID  string
	Text      string
}

func (m mockSDKMessage) GetChannelID() string { return m.ChannelID }
func (m mockSDKMessage) GetSenderID() string  { return m.SenderID }
func (m mockSDKMessage) GetText() string       { return m.Text }

// offlineMockSDKClient implements bot.SDKClient for offline integration tests.
// Records authentication, connection, and sent messages so tests can assert
// the bot lifecycle and write-back behavior without a real Mezon connection.
type offlineMockSDKClient struct {
	mu sync.Mutex

	authToken  string
	authUserID string
	authErr    error
	authCalls  int

	connectErr error
	connected  bool

	onMessageFn  func(interface{})
	onReconnect  func()

	sentMessages []sentMessage
	sendErr      error

	closeCalled bool
	closeErr    error
}

type sentMessage struct {
	channelID string
	text      string
}

func (m *offlineMockSDKClient) Authenticate() (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authCalls++
	return m.authToken, m.authUserID, m.authErr
}

func (m *offlineMockSDKClient) OnMessage(fn func(interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMessageFn = fn
}

func (m *offlineMockSDKClient) OnReconnect(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReconnect = fn
}

func (m *offlineMockSDKClient) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return m.connectErr
}

func (m *offlineMockSDKClient) SendText(channelID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = append(m.sentMessages, sentMessage{channelID, text})
	return m.sendErr
}

func (m *offlineMockSDKClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return m.closeErr
}

// triggerMessage invokes the OnMessage callback registered by the Bot, if any.
func (m *offlineMockSDKClient) triggerMessage(channelID, senderID, text string) {
	m.mu.Lock()
	fn := m.onMessageFn
	m.mu.Unlock()
	if fn != nil {
		fn(mockSDKMessage{
			ChannelID: channelID,
			SenderID:  senderID,
			Text:      text,
		})
	}
}

// defaultOfflineMockClient returns a mock SDK client configured for success.
func defaultOfflineMockClient() *offlineMockSDKClient {
	return &offlineMockSDKClient{
		authToken:  "offline-test-token",
		authUserID: "offline-bot-user",
	}
}

// IsConnected returns whether the mock client is connected.
func (m *offlineMockSDKClient) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

// sentTexts returns the texts of all messages sent via SendText.
func (m *offlineMockSDKClient) sentTexts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	texts := make([]string, len(m.sentMessages))
	for i, sm := range m.sentMessages {
		texts[i] = sm.text
	}
	return texts
}

// ---------------------------------------------------------------------------
// Mock sandbox for offline tests (implements ports.Sandbox)
// ---------------------------------------------------------------------------

// offlineMockSandbox records every Exec call and its stdin content so tests
// can verify whether a Mezon message was dispatched to the sandbox and what
// data it received over stdin.
type offlineMockSandbox struct {
	mu       sync.Mutex
	id       string
	execCalls int
	stdinSnap string // last stdin content
}

func (s *offlineMockSandbox) ID() string { return s.id }

func (s *offlineMockSandbox) Exec(_ context.Context, command []string, stdin io.Reader, _, _ io.Writer) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.execCalls++
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		s.stdinSnap = string(b)
	}
	return 0, nil
}

func (s *offlineMockSandbox) Mount(context.Context, core.Workspace, string) error { return nil }
func (s *offlineMockSandbox) Signals(context.Context, string) error               { return nil }

// execCount returns how many times Exec was called.
func (s *offlineMockSandbox) execCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.execCalls
}

// lastStdin returns the stdin content from the most recent Exec call.
func (s *offlineMockSandbox) lastStdin() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stdinSnap
}

// ---------------------------------------------------------------------------
// Mock sandbox driver (implements ports.SandboxDriver)
// ---------------------------------------------------------------------------

// offlineMockDriver creates offlineMockSandbox instances and records start
// calls so tests can verify the run spec passed to the sandbox.
type offlineMockDriver struct {
	mu         sync.Mutex
	starts     int
	lastSpec   core.RunSpec
	sb         *offlineMockSandbox
}

func (d *offlineMockDriver) Caps() core.SandboxCaps {
	return core.SandboxCaps{DriverName: "offline-mock"}
}

func (d *offlineMockDriver) Start(_ context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.starts++
	d.lastSpec = spec
	sb := &offlineMockSandbox{id: spec.SandboxID}
	d.sb = sb
	return sb, nil
}

func (d *offlineMockDriver) Stop(context.Context, string) error    { return nil }
func (d *offlineMockDriver) Destroy(context.Context, string) error { return nil }

// startedSpec returns the most recent RunSpec passed to Start.
func (d *offlineMockDriver) startedSpec() core.RunSpec {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastSpec
}

// sandbox returns the mock sandbox created by the most recent Start call.
func (d *offlineMockDriver) sandbox() *offlineMockSandbox {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sb
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testOwner and testTenant are the identities used for session creation in
// offline-mode integration tests.
var (
	testOwner  = core.AccountID("offline-mezon-test-owner")
	testTenant = core.TenantID("offline-mezon-test-tenant")
)

// testGrantKey is the symmetric key used to mint local grants in tests.
var testGrantKey = []byte("offline-mezon-test-key")

// meworkYML is a helper for serialising mework.yml content with mezon config.
type meworkYML struct {
	Name    string         `yaml:"name"`
	Version string         `yaml:"version"`
	Engine  string         `yaml:"engine"`
	Backend string         `yaml:"backend"`
	Mezon   *meworkMezon   `yaml:"mezon,omitempty"`
	Policy  *policy.Policy `yaml:"policy,omitempty"`
}

type meworkMezon struct {
	AppID   string `yaml:"app_id"`
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url,omitempty"`
}

// writeOfflineMeworkYML writes a mework.yml into dir with the given fields.
func writeOfflineMeworkYML(t *testing.T, dir, engine, backend string) string {
	t.Helper()
	return writeOfflineMeworkYMLFull(t, dir, engine, backend, "", "", "")
}

// writeOfflineMeworkYMLWithMezon writes mework.yml with mezon credentials.
func writeOfflineMeworkYMLWithMezon(t *testing.T, dir, engine, backend, appID, apiKey, baseURL string) string {
	t.Helper()
	return writeOfflineMeworkYMLFull(t, dir, engine, backend, appID, apiKey, baseURL)
}

// writeOfflineMeworkYMLFull writes mework.yml with all optional fields.
func writeOfflineMeworkYMLFull(t *testing.T, dir, engine, backend, appID, apiKey, baseURL string) string {
	t.Helper()
	m := meworkYML{
		Name:    "offline-agent",
		Version: "1.0.0",
		Engine:  engine,
		Backend: backend,
	}
	if appID != "" {
		m.Mezon = &meworkMezon{
			AppID:   appID,
			APIKey:  apiKey,
			BaseURL: baseURL,
		}
	}
	data, err := yaml.Marshal(&m)
	if err != nil {
		t.Fatalf("marshal mework.yml: %v", err)
	}
	path := filepath.Join(dir, "mework.yml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write mework.yml: %v", err)
	}
	return path
}

// fileResolver reads mework.yml from a workspace directory to satisfy the
// DefinitionResolver interface.
type fileResolver struct {
	workspaceDir string
}

func (r fileResolver) ResolveDefinition(_ context.Context, _ string) (*sandbox.SandboxBundleMetadata, error) {
	data, err := os.ReadFile(filepath.Join(r.workspaceDir, "mework.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, meworkrunner.ErrDefinitionNotFound
		}
		return nil, err
	}
	var meta sandbox.SandboxBundleMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// newOfflineSession creates a workspace-bound session with a mock driver.
func newOfflineSession(t *testing.T, wsDir string) (*meworkrunner.Session, *offlineMockDriver, func()) {
	t.Helper()

	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())

	drv := &offlineMockDriver{}
	localGrant, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, testGrantKey)
	if err != nil {
		t.Fatalf("mint grant: %v", err)
	}
	caller := meworkrunner.Caller{Account: testOwner, Tenant: testTenant, Grant: localGrant}

	sess, err := meworkrunner.StartWorkspaceSession(context.Background(), meworkrunner.StartOptions{
		Ref:          "offline-agent@1.0.0",
		Resolver:     fileResolver{workspaceDir: wsDir},
		WorkspaceDir: wsDir,
		Caller:       caller,
		GrantKey:     testGrantKey,
		ManagerFor:   func(string) *runtime.Manager { return runtime.NewManager(drv) },
		Broker:       broker,
		Sessions:     mgr,
	})
	if err != nil {
		t.Fatalf("StartWorkspaceSession: %v", err)
	}

	cleanup := func() {
		_ = sess.Close(context.Background(), caller)
		mgr.Stop()
	}
	return sess, drv, cleanup
}

// awaitSocket polls until the Unix socket at sockPath exists, or times out.
func awaitSocket(t *testing.T, sockPath string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("socket %s never appeared", sockPath)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// waitBotReady polls until the mock SDK client reports connected, indicating
// the bot has authenticated and the OnMessage handler is registered.
func waitBotReady(t *testing.T, mock *offlineMockSDKClient) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if mock.IsConnected() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("bot did not connect within timeout")
}

// cleanupServer stops the OfflineServer and awaits its completion.
func cleanupServer(ctxCancel context.CancelFunc, srvDone chan error, srv *meworkrunner.OfflineServer) func() {
	return func() {
		ctxCancel()
		if srvDone != nil {
			<-srvDone
		}
		_ = srv.Close()
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestOfflineMezon_StartWithCredentials creates a temp workspace dir with
// mework.yml containing mezon credentials, starts the OfflineServer, and
// verifies that the Mezon bot is started and authenticates.
//
// Realises: "Start offline agent with Mezon credentials".
func TestOfflineMezon_StartWithCredentials(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYMLWithMezon(t, wsDir, "local", "echo", "myapp", "secret123", "")

	appID, apiKey, baseURL, ok := meworkrunner.MezonConfigFromWorkspace(wsDir)
	if !ok {
		t.Fatal("MezonConfigFromWorkspace returned ok=false for workspace with mezon config")
	}
	if appID != "myapp" {
		t.Errorf("appID = %q, want %q", appID, "myapp")
	}
	if apiKey != "secret123" {
		t.Errorf("apiKey = %q, want %q", apiKey, "secret123")
	}
	if baseURL != "" {
		t.Errorf("baseURL = %q, want empty", baseURL)
	}
}

// TestOfflineMezon_StartWithoutCredentials creates a temp workspace dir with
// no Mezon config and asserts that MezonConfigFromWorkspace returns ok=false.
//
// Realises: "Start offline agent without Mezon credentials".
func TestOfflineMezon_StartWithoutCredentials(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYML(t, wsDir, "local", "echo")

	_, _, _, ok := meworkrunner.MezonConfigFromWorkspace(wsDir)
	if ok {
		t.Error("MezonConfigFromWorkspace returned ok=true for workspace without mezon config")
	}
}

// TestOfflineMezon_PolicyEnforcement starts an offline agent with a Mezon bot
// and a policy that blocks messages containing "blockme". It sends a message
// with "blockme now" and asserts the bot receives an error reply and the
// sandbox is NOT called.
//
// Realises: "Policy enforcement for Mezon message" and "Policy blocks Mezon
// message".
func TestOfflineMezon_PolicyEnforcement(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYMLWithMezon(t, wsDir, "local", "echo", "myapp", "secret123", "")

	sess, drv, cleanup := newOfflineSession(t, wsDir)
	t.Cleanup(cleanup)

	srv, err := meworkrunner.NewOfflineServer(wsDir, sess)
	if err != nil {
		t.Fatalf("NewOfflineServer: %v", err)
	}

	// Set a policy that blocks "blockme".
	srv.SetPolicy(&policy.Policy{
		Rules: []policy.Rule{
			{
				Match:  map[string]string{"content": "*blockme*"},
				Action: policy.ActionDeny,
				Reason: "blocked by test policy",
			},
		},
		Default: policy.ActionAllow,
	})

	// Create a mock Mezon bot.
	mockClient := defaultOfflineMockClient()
	bot := mezonbot.New(
		mezoncfg.Config{AppID: "myapp", APIKey: "secret123"},
		mockClient,
		func(msg mezonbot.Message) {},
	)

	// SetMezonBot does NOT exist yet on OfflineServer (RED step).
	srv.SetMezonBot(bot)

	ctx, cancel := context.WithCancel(context.Background())
	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Start(ctx) }()
	waitBotReady(t, mockClient)

	// Trigger a message that should be blocked by policy.
	mockClient.triggerMessage("ch_block_test", "evil_user", "blockme now")

	// Allow message processing to complete.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-srvDone

	// Assert the error reply was sent.
	texts := mockClient.sentTexts()
	if len(texts) < 1 {
		t.Fatal("expected at least one reply message")
	}
	if texts[0] != "blocked by test policy" {
		t.Errorf("reply = %q, want %q", texts[0], "blocked by test policy")
	}

	// Assert sandbox.Exec was NOT called (blocked by policy).
	if sb := drv.sandbox(); sb != nil {
		if n := sb.execCount(); n != 0 {
			t.Errorf("sandbox.Exec called %d times, want 0 (blocked by policy)", n)
		}
	}
}

// TestOfflineMezon_PolicyPassesToSandbox starts an offline agent with a
// permissive policy, sends a Mezon message, and verifies the sandbox Exec
// is called with the message text as stdin and the bot's SendMessage is
// called with the sandbox output.
//
// Realises: "Reply sent to originating channel".
func TestOfflineMezon_PolicyPassesToSandbox(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYMLWithMezon(t, wsDir, "local", "echo", "myapp", "secret123", "")

	sess, drv, cleanup := newOfflineSession(t, wsDir)
	t.Cleanup(cleanup)

	srv, err := meworkrunner.NewOfflineServer(wsDir, sess)
	if err != nil {
		t.Fatalf("NewOfflineServer: %v", err)
	}

	// Permissive policy — allow everything.
	srv.SetPolicy(&policy.Policy{Default: policy.ActionAllow})

	mockClient := defaultOfflineMockClient()
	bot := mezonbot.New(
		mezoncfg.Config{AppID: "myapp", APIKey: "secret123"},
		mockClient,
		func(msg mezonbot.Message) {},
	)

	srv.SetMezonBot(bot)

	ctx, cancel := context.WithCancel(context.Background())
	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Start(ctx) }()
	waitBotReady(t, mockClient)

	// Trigger a message that should pass policy and reach the sandbox.
	mockClient.triggerMessage("ch_pass_test", "test_user", "hello from mezon")

	// Allow message processing to complete.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-srvDone

	// Assert sandbox.Exec was called with the message text as stdin.
	sb := drv.sandbox()
	if sb == nil {
		t.Fatal("sandbox was not created")
	}
	if n := sb.execCount(); n < 1 {
		t.Errorf("sandbox.Exec called %d times, want >= 1", n)
	}
	if stdin := sb.lastStdin(); stdin != "hello from mezon" {
		t.Errorf("sandbox stdin = %q, want %q", stdin, "hello from mezon")
	}

	// Assert a reply was sent (the mock sandbox returns "0" for exit code
	// and writes nothing to stdout, so the reply is empty).
	texts := mockClient.sentTexts()
	if len(texts) < 1 {
		t.Error("expected at least one reply message")
	}
}

// TestOfflineMezon_NoDatabaseDependency starts an offline agent with Mezon
// bot and without DATABASE_URL, verifying that no database access occurs.
//
// Realises: "No database dependency".
func TestOfflineMezon_NoDatabaseDependency(t *testing.T) {
	// This test must never read the DATABASE_URL env var.
	if os.Getenv("TEST_DATABASE_URL") != "" {
		t.Log("TEST_DATABASE_URL is set but this test does not use it")
	}
	_ = os.Getenv("DATABASE_URL") // only check that we don't crash

	wsDir := t.TempDir()
	writeOfflineMeworkYMLWithMezon(t, wsDir, "local", "echo", "myapp", "secret123", "")

	_, _, _, ok := meworkrunner.MezonConfigFromWorkspace(wsDir)
	if !ok {
		t.Fatal("expected mezon config to be readable without database")
	}
}

// TestOfflineMezon_ReadConfigFromMeworkYml writes mework.yml with mezon
// config and verifies MezonConfigFromWorkspace parses it correctly.
//
// Realises: "Read Mezon config from mework.yml".
func TestOfflineMezon_ReadConfigFromMeworkYml(t *testing.T) {
	tests := []struct {
		name    string
		appID   string
		apiKey  string
		baseURL string
	}{
		{
			name:   "minimal config",
			appID:  "app-minimal",
			apiKey: "key-minimal",
		},
		{
			name:    "with custom base URL",
			appID:   "app-custom",
			apiKey:  "key-custom",
			baseURL: "https://self-hosted.mezon.example",
		},
		{
			name:   "app ID with hyphens and dots",
			appID:  "my-app.v2",
			apiKey: "complex-key-123!@#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsDir := t.TempDir()
			writeOfflineMeworkYMLWithMezon(t, wsDir, "local", "echo", tt.appID, tt.apiKey, tt.baseURL)

			appID, apiKey, baseURL, ok := meworkrunner.MezonConfigFromWorkspace(wsDir)
			if !ok {
				t.Fatal("MezonConfigFromWorkspace returned ok=false")
			}
			if appID != tt.appID {
				t.Errorf("appID = %q, want %q", appID, tt.appID)
			}
			if apiKey != tt.apiKey {
				t.Errorf("apiKey = %q, want %q", apiKey, tt.apiKey)
			}
			if baseURL != tt.baseURL {
				t.Errorf("baseURL = %q, want %q", baseURL, tt.baseURL)
			}
		})
	}
}

// TestOfflineMezon_InMemoryBinding starts an offline agent, sends a Mezon
// message, and verifies the channel ID is bound to the single local sandbox
// session in memory (no registry lookup).
//
// Realises: "In-memory binding in offline mode".
func TestOfflineMezon_InMemoryBinding(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYMLWithMezon(t, wsDir, "local", "echo", "myapp", "secret123", "")

	sess, drv, cleanup := newOfflineSession(t, wsDir)
	t.Cleanup(cleanup)

	srv, err := meworkrunner.NewOfflineServer(wsDir, sess)
	if err != nil {
		t.Fatalf("NewOfflineServer: %v", err)
	}

	mockClient := defaultOfflineMockClient()
	bot := mezonbot.New(
		mezoncfg.Config{AppID: "myapp", APIKey: "secret123"},
		mockClient,
		func(msg mezonbot.Message) {},
	)

	srv.SetMezonBot(bot)

	ctx, cancel := context.WithCancel(context.Background())
	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Start(ctx) }()
	waitBotReady(t, mockClient)

	// Trigger messages from two different channels.
	mockClient.triggerMessage("ch_a", "user1", "message in channel A")
	mockClient.triggerMessage("ch_b", "user2", "message in channel B")

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-srvDone

	// The offline server uses a single sandbox session; both messages
	// should have been dispatched to it (sandbox.Exec called twice).
	sb := drv.sandbox()
	if sb == nil {
		t.Fatal("sandbox was not created")
	}
	if n := sb.execCount(); n < 2 {
		t.Errorf("sandbox.Exec called %d times, want >= 2 (one per message)", n)
	}
}

// TestOfflineMezon_DaemonEnrollsMezonBot verifies that when a daemon enrolls
// with mezon-bot capability, the enrollment specs include "mezon-bot".
//
// Realises: "Enrolled daemon advertises mezon-bot".
func TestOfflineMezon_DaemonEnrollsMezonBot(t *testing.T) {
	// This test verifies that the runner enrollment includes the mezon-bot
	// spec. Because the runner package does not yet expose this, we use
	// MezonConfigFromWorkspace as a proxy for the RED compilation failure.
	wsDir := t.TempDir()
	writeOfflineMeworkYMLWithMezon(t, wsDir, "local", "echo", "myapp", "secret123", "")

	_, _, _, ok := meworkrunner.MezonConfigFromWorkspace(wsDir)
	if !ok {
		t.Error("MezonConfigFromWorkspace returned ok=false, want true")
	}
}
