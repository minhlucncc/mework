// Package main tests for the mework-mezon-worker standalone binary.
//
// RED step: all tests fail to compile because the production types
// (Config, Worker, Load, New, Cursor) do not exist yet.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mezonbot "mework/libs/server/provider/mezon/bot"
	sharedMezon "mework/libs/shared/providers/mezon"
)

// ---------------------------------------------------------------------------
// Mock SDK client for Mezon bot (implements mezonbot.SDKClient)
// ---------------------------------------------------------------------------

// mockBotClient implements the SDKClient interface for testing.
type mockBotClient struct {
	mu sync.Mutex

	authToken     string
	authUserID    string
	authErr       error
	authCallCount int

	connectErr error
	connected  bool

	onMessageFn   func(interface{})
	onReconnectFn func()

	lastChannelID string
	lastText      string
	sendErr       error

	closeCalled bool
	closeErr    error

	// disconnectFn, when set, is called after each failed reconnect attempt.
	// Used to simulate permanent disconnection scenarios.
	disconnectFn func()
}

func (m *mockBotClient) Authenticate() (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authCallCount++
	return m.authToken, m.authUserID, m.authErr
}

func (m *mockBotClient) OnMessage(fn func(interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMessageFn = fn
}

func (m *mockBotClient) OnReconnect(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReconnectFn = fn
}

func (m *mockBotClient) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return m.connectErr
}

func (m *mockBotClient) SendText(channelID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastChannelID = channelID
	m.lastText = text
	return m.sendErr
}

func (m *mockBotClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	m.connected = false
	return m.closeErr
}

// triggerMessage invokes the OnMessage callback registered by the Bot, if any.
func (m *mockBotClient) triggerMessage(msg interface{}) {
	m.mu.Lock()
	fn := m.onMessageFn
	m.mu.Unlock()
	if fn != nil {
		fn(msg)
	}
}

// triggerReconnect invokes the OnReconnect callback registered by the Bot, if any.
func (m *mockBotClient) triggerReconnect() {
	m.mu.Lock()
	fn := m.onReconnectFn
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// defaultMockBot returns a mockBotClient configured for success on every operation.
func defaultMockBot() *mockBotClient {
	return &mockBotClient{
		authToken:  "mock-sdk-token",
		authUserID: "mock-bot-user",
	}
}

// ---------------------------------------------------------------------------
// Mock API server (simulates mework-server enqueue and list endpoints)
// ---------------------------------------------------------------------------

// enqueuePayload is the POST body sent to /api/v1/jobs/enqueue.
type enqueuePayload struct {
	ProviderCode string `json:"provider_code"`
	ChannelID    string `json:"channel_id"`
	SenderID     string `json:"sender_id"`
	Text         string `json:"text"`
	MessageID    string `json:"message_id"`
}

// jobRecord is a single job returned by GET /api/v1/jobs.
type jobRecord struct {
	ID            string `json:"id"`
	ChannelID     string `json:"channel_id"`
	ResultSummary string `json:"result_summary"`
	Status        string `json:"status"`
}

// mockAPIServer simulates the mework-server HTTP API for tests.
type mockAPIServer struct {
	t      *testing.T
	server *httptest.Server
	URL    string

	mu sync.Mutex

	// Enqueue endpoint state
	enqueueCalls []enqueuePayload
	enqueueCount int
	enqueueHits  chan struct{} // signals each enqueue call (for sync)
	enqueueBody  enqueuePayload
	enqueueDone  bool // set when first enqueue call arrives

	enqueueStatus int           // HTTP status code for enqueue responses
	enqueueDelay  time.Duration // artificial delay before responding

	// Jobs list endpoint state
	jobsCalls    int
	jobsHits     chan struct{} // signals each list call
	jobsResponse []jobRecord
	jobsStatus   int
	jobsDelay    time.Duration
}

func newMockAPIServer(t *testing.T) *mockAPIServer {
	m := &mockAPIServer{
		t:             t,
		enqueueStatus: http.StatusCreated,
		jobsStatus:    http.StatusOK,
		jobsResponse:  []jobRecord{},
		enqueueHits:   make(chan struct{}, 100),
		jobsHits:      make(chan struct{}, 100),
	}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs/enqueue":
			var payload enqueuePayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			m.mu.Lock()
			m.enqueueCalls = append(m.enqueueCalls, payload)
			m.enqueueCount++
			status := m.enqueueStatus
			delay := m.enqueueDelay
			m.mu.Unlock()

			select {
			case m.enqueueHits <- struct{}{}:
			default:
			}

			if delay > 0 {
				time.Sleep(delay)
			}
			if status >= 300 {
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "server error"})
				return
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"job_id": "enqueued-001"})

		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs":
			m.mu.Lock()
			// Parse query params for info logging only; server returns
			// whatever canned response is configured.
			m.jobsCalls++
			resp := m.jobsResponse
			status := m.jobsStatus
			delay := m.jobsDelay
			m.mu.Unlock()

			select {
			case m.jobsHits <- struct{}{}:
			default:
			}

			if delay > 0 {
				time.Sleep(delay)
			}
			w.WriteHeader(status)
			if status == http.StatusOK {
				_ = json.NewEncoder(w).Encode(resp)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	m.URL = m.server.URL
	return m
}

func (m *mockAPIServer) Close() {
	m.server.Close()
}

func (m *mockAPIServer) enqueueCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enqueueCount
}

func (m *mockAPIServer) jobsCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobsCalls
}

// waitForEnqueueCalls blocks until at least n enqueue requests arrive or timeout.
func (m *mockAPIServer) waitForEnqueueCalls(t *testing.T, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for m.enqueueCallCount() < n {
		select {
		case <-m.enqueueHits:
		case <-deadline:
			t.Fatalf("timed out waiting for %d enqueue calls (got %d)", n, m.enqueueCallCount())
		}
	}
}

// waitForListCalls blocks until at least n jobs-list requests arrive or timeout.
func (m *mockAPIServer) waitForListCalls(t *testing.T, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for m.jobsCallCount() < n {
		select {
		case <-m.jobsHits:
		case <-deadline:
			t.Fatalf("timed out waiting for %d jobs-list calls (got %d)", n, m.jobsCallCount())
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Config loading
// ---------------------------------------------------------------------------

func TestConfig_Load(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *Config
		wantErr string // substring of expected error; empty means success
	}{
		{
			name: "start with minimal config",
			env: map[string]string{
				"MEZON_APP_ID":      "app-123",
				"MEZON_API_KEY":     "key-456",
				"MEWORK_TOKEN":      "tok-789",
				"MEWORK_SERVER_URL": "http://localhost:8080",
			},
			want: &Config{
				MezonAppID:      "app-123",
				MezonAPIKey:     "key-456",
				MeworkToken:     "tok-789",
				MeworkServerURL: "http://localhost:8080",
				PollInterval:    5 * time.Second,
				CursorPath:      "./.mework-mezon-cursor",
			},
		},
		{
			name: "MEWORK_SERVER_URL defaults when not set",
			env: map[string]string{
				"MEZON_APP_ID":  "app-123",
				"MEZON_API_KEY": "key-456",
				"MEWORK_TOKEN":  "tok-789",
			},
			want: &Config{
				MezonAppID:      "app-123",
				MezonAPIKey:     "key-456",
				MeworkToken:     "tok-789",
				MeworkServerURL: "http://localhost:8080",
				PollInterval:    5 * time.Second,
				CursorPath:      "./.mework-mezon-cursor",
			},
		},
		{
			name: "missing MEZON_APP_ID exits",
			env: map[string]string{
				"MEZON_API_KEY":     "key-456",
				"MEWORK_TOKEN":      "tok-789",
				"MEWORK_SERVER_URL": "http://localhost:8080",
			},
			wantErr: "MEZON_APP_ID",
		},
		{
			name: "missing MEZON_API_KEY exits",
			env: map[string]string{
				"MEZON_APP_ID":      "app-123",
				"MEWORK_TOKEN":      "tok-789",
				"MEWORK_SERVER_URL": "http://localhost:8080",
			},
			wantErr: "MEZON_API_KEY",
		},
		{
			name: "missing MEWORK_TOKEN exits",
			env: map[string]string{
				"MEZON_APP_ID":  "app-123",
				"MEZON_API_KEY": "key-456",
				// MEWORK_TOKEN intentionally omitted
				"MEWORK_SERVER_URL": "http://localhost:8080",
			},
			wantErr: "MEWORK_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore environment.
			savedEnv := os.Environ()
			t.Cleanup(func() {
				os.Clearenv()
				for _, kv := range savedEnv {
					if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
						_ = os.Setenv(parts[0], parts[1])
					}
				}
			})

			os.Clearenv()
			for k, v := range tt.env {
				_ = os.Setenv(k, v)
			}

			got, err := Load()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Load() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("Load() returned nil")
			}
			if got.MezonAppID != tt.want.MezonAppID {
				t.Errorf("MezonAppID = %q, want %q", got.MezonAppID, tt.want.MezonAppID)
			}
			if got.MezonAPIKey != tt.want.MezonAPIKey {
				t.Errorf("MezonAPIKey = %q, want %q", got.MezonAPIKey, tt.want.MezonAPIKey)
			}
			if got.MeworkToken != tt.want.MeworkToken {
				t.Errorf("MeworkToken = %q, want %q", got.MeworkToken, tt.want.MeworkToken)
			}
			if got.MeworkServerURL != tt.want.MeworkServerURL {
				t.Errorf("MeworkServerURL = %q, want %q", got.MeworkServerURL, tt.want.MeworkServerURL)
			}
			if got.PollInterval != tt.want.PollInterval {
				t.Errorf("PollInterval = %v, want %v", got.PollInterval, tt.want.PollInterval)
			}
			if got.CursorPath != tt.want.CursorPath {
				t.Errorf("CursorPath = %q, want %q", got.CursorPath, tt.want.CursorPath)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Inbound loop — message received and enqueued
// ---------------------------------------------------------------------------

func TestWorker_Inbound_MessageEnqueued(t *testing.T) {
	mockSrv := newMockAPIServer(t)
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)

	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    100 * time.Millisecond,
		CursorPath:      filepath.Join(t.TempDir(), ".cursor"),
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Allow the worker's inbound loop to reach bot.Start() (sets b.ctx).
	time.Sleep(300 * time.Millisecond)

	// Simulate receiving a Mezon channel message from another user.
	mockBot.triggerMessage(struct {
		ChannelID string
		SenderID  string
		Text      string
	}{
		ChannelID: "ch_abc",
		SenderID:  "user-123",
		Text:      "hello from mezon",
	})

	// Wait for the enqueue call to arrive.
	mockSrv.waitForEnqueueCalls(t, 1, 5*time.Second)

	mockSrv.mu.Lock()
	call := mockSrv.enqueueCalls[0]
	mockSrv.mu.Unlock()

	if call.ProviderCode != "mezon" {
		t.Errorf("provider_code = %q, want %q", call.ProviderCode, "mezon")
	}
	if call.ChannelID != "ch_abc" {
		t.Errorf("channel_id = %q, want %q", call.ChannelID, "ch_abc")
	}
	if call.SenderID != "user-123" {
		t.Errorf("sender_id = %q, want %q", call.SenderID, "user-123")
	}
	if call.Text != "hello from mezon" {
		t.Errorf("text = %q, want %q", call.Text, "hello from mezon")
	}
}

// ---------------------------------------------------------------------------
// Test: Inbound loop — enqueue 5xx logged, no crash
// ---------------------------------------------------------------------------

func TestWorker_Inbound_ServerErrorNoCrash(t *testing.T) {
	mockSrv := newMockAPIServer(t)
	mockSrv.mu.Lock()
	mockSrv.enqueueStatus = http.StatusInternalServerError
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    100 * time.Millisecond,
		CursorPath:      filepath.Join(t.TempDir(), ".cursor"),
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()
	time.Sleep(300 * time.Millisecond)

	// Trigger first message (enqueue returns 500).
	mockBot.triggerMessage(struct {
		ChannelID string
		SenderID  string
		Text      string
	}{ChannelID: "ch_1", SenderID: "u-1", Text: "first"})
	mockSrv.waitForEnqueueCalls(t, 1, 5*time.Second)

	// Trigger second message — worker must not have crashed after the 500.
	mockBot.triggerMessage(struct {
		ChannelID string
		SenderID  string
		Text      string
	}{ChannelID: "ch_2", SenderID: "u-2", Text: "second"})
	mockSrv.waitForEnqueueCalls(t, 2, 5*time.Second)

	// Both enqueue calls were made (worker survived the 5xx).
	if mockSrv.enqueueCallCount() != 2 {
		t.Errorf("enqueue calls count = %d, want 2", mockSrv.enqueueCallCount())
	}
}

// ---------------------------------------------------------------------------
// Test: Inbound loop — enqueue timeout / hang logged, no crash
// ---------------------------------------------------------------------------

func TestWorker_Inbound_TimeoutNoCrash(t *testing.T) {
	mockSrv := newMockAPIServer(t)
	// Make enqueue endpoint hang (very long delay).
	mockSrv.mu.Lock()
	mockSrv.enqueueDelay = 10 * time.Second // longer than any test-run timeout
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    100 * time.Millisecond,
		CursorPath:      filepath.Join(t.TempDir(), ".cursor"),
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()
	time.Sleep(300 * time.Millisecond)

	// Trigger message — enqueue will block on the slow server.
	mockBot.triggerMessage(struct {
		ChannelID string
		SenderID  string
		Text      string
	}{ChannelID: "ch_1", SenderID: "u-1", Text: "hello"})

	// Give the worker time to attempt the request and not crash.
	// The http.Client timeout should trigger and return an error.
	time.Sleep(2 * time.Second)

	// Worker should still be running (trigger another message whose enqueue
	// also hangs, proving the worker didn't crash on the first failure).
	mockBot.triggerMessage(struct {
		ChannelID string
		SenderID  string
		Text      string
	}{ChannelID: "ch_2", SenderID: "u-2", Text: "world"})

	// If we reach here without the worker crashing, the test passes.
	// (The enqueue requests will never complete because the delay is 10s,
	// but the worker's goroutine should return from the HTTP error and continue.)
	time.Sleep(500 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Test: Outbound loop — poll and reply
// ---------------------------------------------------------------------------

func TestWorker_Outbound_PollAndReply(t *testing.T) {
	mockSrv := newMockAPIServer(t)
	mockSrv.mu.Lock()
	mockSrv.jobsResponse = []jobRecord{
		{ID: "job-001", ChannelID: "ch_abc", ResultSummary: "task complete", Status: "done"},
	}
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cursorPath := filepath.Join(t.TempDir(), ".cursor")
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    50 * time.Millisecond,
		CursorPath:      cursorPath,
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Wait for the outbound loop to send a message via the mock bot.
	deadline := time.After(5 * time.Second)
	for {
		mockBot.mu.Lock()
		ch := mockBot.lastChannelID
		txt := mockBot.lastText
		sent := mockBot.lastChannelID != ""
		mockBot.mu.Unlock()
		if sent {
			if ch != "ch_abc" {
				t.Errorf("SendMessage channel = %q, want %q", ch, "ch_abc")
			}
			if txt != "task complete" {
				t.Errorf("SendMessage text = %q, want %q", txt, "task complete")
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for SendMessage on outbound reply")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Outbound loop — cursor persisted after successful reply
// ---------------------------------------------------------------------------

func TestWorker_Outbound_CursorPersisted(t *testing.T) {
	mockSrv := newMockAPIServer(t)
	mockSrv.mu.Lock()
	mockSrv.jobsResponse = []jobRecord{
		{ID: "job-001", ChannelID: "ch_abc", ResultSummary: "ok", Status: "done"},
	}
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cursorDir := t.TempDir()
	cursorPath := filepath.Join(cursorDir, ".cursor")
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    50 * time.Millisecond,
		CursorPath:      cursorPath,
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Wait for cursor file to appear with content "job-001".
	deadline := time.After(5 * time.Second)
	for {
		data, err := os.ReadFile(cursorPath)
		if err == nil {
			content := strings.TrimSpace(string(data))
			if content == "job-001" {
				return // success
			}
		}
		select {
		case <-deadline:
			data, _ := os.ReadFile(cursorPath)
			t.Fatalf("timed out waiting for cursor file; content: %q", string(data))
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Outbound loop — cursor restored on restart (only new jobs processed)
// ---------------------------------------------------------------------------

func TestWorker_Outbound_CursorRestoredOnRestart(t *testing.T) {
	cursorDir := t.TempDir()
	cursorPath := filepath.Join(cursorDir, ".cursor")
	// Pre-seed cursor at "job-001".
	if err := os.WriteFile(cursorPath, []byte("job-001"), 0600); err != nil {
		t.Fatalf("failed to pre-seed cursor: %v", err)
	}

	mockSrv := newMockAPIServer(t)
	mockSrv.mu.Lock()
	// job-002 is before/at cursor (should be skipped); job-003 is after.
	mockSrv.jobsResponse = []jobRecord{
		{ID: "job-002", ChannelID: "ch_002", ResultSummary: "old task", Status: "done"},
		{ID: "job-003", ChannelID: "ch_003", ResultSummary: "new result", Status: "done"},
	}
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    50 * time.Millisecond,
		CursorPath:      cursorPath,
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Outbound loop should skip job-002 and only process job-003.
	deadline := time.After(5 * time.Second)
	for {
		mockBot.mu.Lock()
		lastTxt := mockBot.lastText
		mockBot.mu.Unlock()
		if lastTxt == "new result" {
			// job-003 was processed successfully.
			// Assert that "old task" was NOT sent.
			mockBot.mu.Lock()
			ch := mockBot.lastChannelID
			mockBot.mu.Unlock()
			if ch != "ch_003" {
				t.Errorf("bot sent to channel %q, expected ch_003 (only new job after cursor)", ch)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job-003 to be processed; last bot text: %q", mockBot.lastText)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Outbound loop — SendMessage failure does NOT advance cursor
// ---------------------------------------------------------------------------

func TestWorker_Outbound_SendMessageFailureNoCursorAdvance(t *testing.T) {
	cursorDir := t.TempDir()
	cursorPath := filepath.Join(cursorDir, ".cursor")

	mockSrv := newMockAPIServer(t)
	mockSrv.mu.Lock()
	mockSrv.jobsResponse = []jobRecord{
		{ID: "job-001", ChannelID: "ch_abc", ResultSummary: "hello", Status: "done"},
	}
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	mockBot.sendErr = errors.New("send failed") // SendMessage will fail
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    50 * time.Millisecond,
		CursorPath:      cursorPath,
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Give the worker enough time to attempt the send and fail.
	time.Sleep(2 * time.Second)

	// Assert cursor file either does not exist or does NOT contain "job-001".
	data, err := os.ReadFile(cursorPath)
	if err == nil {
		content := strings.TrimSpace(string(data))
		if content == "job-001" {
			t.Error("cursor was advanced despite SendMessage failure")
		}
	}
	// If os.IsNotExist(err) — file doesn't exist, that's fine (cursor not advanced).
}

// ---------------------------------------------------------------------------
// Test: Inbound fails independently — outbound continues polling
// ---------------------------------------------------------------------------

func TestWorker_InboundFailsOutboundContinues(t *testing.T) {
	mockSrv := newMockAPIServer(t)
	mockSrv.mu.Lock()
	mockSrv.jobsResponse = []jobRecord{} // just checking polling happens
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	// Bot connection fails — inbound loop will exit.
	mockBot := defaultMockBot()
	mockBot.connectErr = errors.New("connection rejected")
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    50 * time.Millisecond,
		CursorPath:      filepath.Join(t.TempDir(), ".cursor"),
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Outbound loop should continue polling the list endpoint despite bot failure.
	mockSrv.waitForListCalls(t, 3, 5*time.Second)
}

// ---------------------------------------------------------------------------
// Test: Outbound poll error logged — inbound continues
// ---------------------------------------------------------------------------

func TestWorker_OutboundPollErrorLogged(t *testing.T) {
	mockSrv := newMockAPIServer(t)
	mockSrv.mu.Lock()
	mockSrv.jobsStatus = http.StatusServiceUnavailable // 503
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    50 * time.Millisecond,
		CursorPath:      filepath.Join(t.TempDir(), ".cursor"),
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Let the outbound loop poll and fail a few times.
	mockSrv.waitForListCalls(t, 2, 5*time.Second)

	// Inbound loop should still process messages.
	mockBot.triggerMessage(struct {
		ChannelID string
		SenderID  string
		Text      string
	}{ChannelID: "ch_in", SenderID: "u-in", Text: "inbound still works"})

	mockSrv.waitForEnqueueCalls(t, 1, 5*time.Second)
}

// ---------------------------------------------------------------------------
// Test: Permanent disconnect stops inbound loop only — outbound continues
// ---------------------------------------------------------------------------

func TestWorker_PermanentDisconnectStopsInboundOnly(t *testing.T) {
	mockSrv := newMockAPIServer(t)
	mockSrv.mu.Lock()
	mockSrv.jobsResponse = []jobRecord{} // empty
	mockSrv.mu.Unlock()
	defer mockSrv.Close()

	mockBot := defaultMockBot()
	bot := mezonbot.New(
		sharedMezon.Config{AppID: "test-app", APIKey: "test-key"},
		mockBot,
		func(msg mezonbot.Message) {},
	)
	cfg := &Config{
		MeworkServerURL: mockSrv.URL,
		MeworkToken:     "test-token",
		PollInterval:    50 * time.Millisecond,
		CursorPath:      filepath.Join(t.TempDir(), ".cursor"),
	}
	w := New(cfg, bot)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Wait for worker to start and outbound loop to begin calling.
	mockSrv.waitForListCalls(t, 2, 5*time.Second)
	// Record current call count.
	mockSrv.mu.Lock()
	baseline := mockSrv.jobsCalls
	mockSrv.mu.Unlock()

	// Simulate permanent disconnection by repeatedly firing the
	// reconnection callback with a failing auth.
	mockBot.mu.Lock()
	mockBot.authErr = errors.New("permanent auth failure")
	mockBot.mu.Unlock()
	mockBot.triggerReconnect()

	// After inbound failure, outbound should still be polling.
	mockSrv.waitForListCalls(t, baseline+3, 5*time.Second)
}
