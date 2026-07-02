package runner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mework/libs/shared/policy"
	"mework/libs/sandbox"
)

// ChatEntry records one turn in the conversation history.
type ChatEntry struct {
	Role    string // "user" or "assistant"
	Content string
}

// OfflineServer listens on a Unix socket, accepts JSON-RPC messages, and
// dispatches tasks to a workspace-bound session's sandbox over stdin (never
// argv), preserving the injection-safety invariant. Conversation history is
// accumulated and injected on each call so the backend sees prior context.
type OfflineServer struct {
	socketPath  string
	listener    net.Listener
	session     *Session
	done        chan struct{}
	mu          sync.Mutex
	closed      bool
	policy      *policy.Policy
	rateLimiter *policy.RateLimiter

	// Conversation history — accumulated across calls and injected into each
	// prompt so the backend sees prior context despite one-shot execution.
	history   []ChatEntry
	histMu    sync.Mutex
}

const (
	// maxHistoryTurns caps the number of conversation turns retained.
	maxHistoryTurns = 50
	// maxHistoryChars caps the total characters of the formatted history
	// transcript to avoid overflowing the context window.
	maxHistoryChars = 8000
)

// ---------------------------------------------------------------------------
// Socket path derivation
// ---------------------------------------------------------------------------

// SocketPath returns the deterministic Unix socket path for a workspace
// directory, derived from its SHA-256 hash.  Empty directories return an
// error.  Trailing slashes are normalised before hashing so that
// "/tmp/ws" and "/tmp/ws/" produce the same path.
func SocketPath(workspaceDir string) (string, error) {
	if workspaceDir == "" {
		return "", fmt.Errorf("workspace directory must not be empty")
	}
	normalised := strings.TrimRight(workspaceDir, "/")
	hash := sha256.Sum256([]byte(normalised))
	return fmt.Sprintf("/tmp/mework-offline-%x.sock", hash), nil
}

// ---------------------------------------------------------------------------
// Server lifecycle
// ---------------------------------------------------------------------------

// NewOfflineServer creates a new OfflineServer bound to the given workspace
// directory.  The session must already have been started (via
// StartWorkspaceSession or OpenSession).
// SetPolicy attaches a message policy to the server. When set, every
// incoming "run" request is checked against the policy before execution.
func (s *OfflineServer) SetPolicy(p *policy.Policy) {
	s.policy = p
	s.rateLimiter = policy.NewRateLimiter()
}

func NewOfflineServer(workspaceDir string, session *Session) (*OfflineServer, error) {
	sockPath, err := SocketPath(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("offline server: %w", err)
	}
	return &OfflineServer{
		socketPath: sockPath,
		session:    session,
		done:       make(chan struct{}),
	}, nil
}

// buildPrompt assembles the full prompt from conversation history and the
// current instruction. The history is formatted as a transcript so the
// backend sees the full context despite each call being one-shot.
func (s *OfflineServer) buildPrompt(instruction string) string {
	s.histMu.Lock()
	defer s.histMu.Unlock()

	// Format history as a conversation transcript.
	var transcript strings.Builder
	for _, entry := range s.history {
		role := strings.Title(entry.Role)
		transcript.WriteString(fmt.Sprintf("%s: %s\n", role, entry.Content))
	}

	// Trim from the front if the transcript is too long.
	hist := transcript.String()
	if len(hist) > maxHistoryChars {
		hist = trimFront(hist, maxHistoryChars)
	}

	// Assemble the final prompt.
	var prompt strings.Builder
	if hist != "" {
		prompt.WriteString("Previous conversation:\n")
		prompt.WriteString(hist)
		prompt.WriteString("\n")
	}
	prompt.WriteString(fmt.Sprintf("User: %s\nAssistant:", instruction))
	return prompt.String()
}

// trimFront returns the last n characters of s, starting at a newline boundary
// when possible so the transcript doesn't begin mid-line.
func trimFront(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	// Try to start at a newline boundary.
	if i := strings.IndexByte(s[cut:], '\n'); i >= 0 && i < n/2 {
		cut += i + 1
	}
	return s[cut:]
}

// appendExchange adds one user→assistant exchange to the conversation history,
// trimming the oldest entries when the history is full.
func (s *OfflineServer) appendExchange(instruction, response string) {
	s.histMu.Lock()
	defer s.histMu.Unlock()

	s.history = append(s.history,
		ChatEntry{Role: "user", Content: instruction},
		ChatEntry{Role: "assistant", Content: response},
	)

	// Trim old entries when we exceed the cap.
	if len(s.history) > maxHistoryTurns*2 {
		excess := len(s.history) - maxHistoryTurns*2
		s.history = s.history[excess:]
	}
}

// Start unlinks any stale socket at the path, begins listening, and accepts
// connections in a background goroutine.  It blocks until ctx is cancelled
// or a fatal error occurs.
func (s *OfflineServer) Start(ctx context.Context) error {
	// Unlink any stale socket from a previous run.
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket %s: %w", s.socketPath, err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}
	// Restrict socket access to the owning user.
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("chmod %s: %w", s.socketPath, err)
	}
	s.listener = listener

	// Start the accept loop in the background.
	go s.acceptLoop(ctx)

	// Block until the context is cancelled (graceful shutdown).
	<-ctx.Done()
	_ = s.listener.Close()
	return ctx.Err()
}

// Close removes the socket file and marks the server as shut down.  It is
// safe to call multiple times.
func (s *OfflineServer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove socket file %s: %w", s.socketPath, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Connection handling — JSON-RPC over Unix socket
// ---------------------------------------------------------------------------

// acceptLoop accepts connections and dispatches each in its own goroutine.
func (s *OfflineServer) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Listener was closed (shutdown) or a transient error occurred.
			return
		}
		go s.handleConnection(ctx, conn)
	}
}

// jsonRPCRequest is a minimal JSON-RPC 2.0 request body.
type jsonRPCRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     interface{}     `json:"id"`
}

// jsonRPCResponse is a minimal JSON-RPC 2.0 response body.
type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// runParams is the expected params for the "run" method.
type runParams struct {
	Instruction string `json:"instruction"`
	Sender      string `json:"sender,omitempty"`
}

// runResult is the result returned for a successful "run" invocation.
type runResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// handleConnection reads one JSON-RPC request and dispatches it.  Only the
// "run" method is supported.
func (s *OfflineServer) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	var req jsonRPCRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	switch req.Method {
	case "run":
		var params runParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendJSONRPCError(conn, req.ID, -32602, "invalid params")
			return
		}

		// ---- POLICY ENFORCEMENT ----
		if s.policy != nil {
			sender := params.Sender
			if sender == "" {
				sender = "anonymous"
			}
			attrs := policy.Attributes{
				"sender":          sender,
				"authenticated":   "true",
				"content":         params.Instruction,
				"content_length":  fmt.Sprint(len(params.Instruction)),
				"time":            time.Now().UTC().Format(time.RFC3339),
				"channel":         "local",
			}
			result, err := s.policy.Enforce(attrs)
			if err != nil {
				sendJSONRPCError(conn, req.ID, -32603, "policy error: "+err.Error())
				return
			}
			if !result.Allowed {
				sendJSONRPCError(conn, req.ID, -32001, result.Reason)
				return
			}
			// Rate limit check
			if result.Reason != "" {
				if count, ok := policy.ParseLimit(result.Reason); ok {
					if !s.rateLimiter.Allow(sender, count) {
						sendJSONRPCError(conn, req.ID, -32002, "rate limit exceeded")
						return
					}
				}
			}
		}
		// ---- END POLICY ENFORCEMENT ----

		s.handleRun(ctx, conn, req.ID, params.Instruction)
	default:
		sendJSONRPCError(conn, req.ID, -32601, "method not found")
	}
}

// handleRun feeds the instruction to the sandbox over stdin and returns the
// output. Conversation history is injected into the prompt so the backend
// (e.g. claude) sees prior context despite each call being a fresh process.
func (s *OfflineServer) handleRun(ctx context.Context, conn net.Conn, id interface{}, instruction string) {
	// Build the prompt with conversation history injected.
	prompt := s.buildPrompt(instruction)

	// Resolve the backend command with appropriate flags for non-interactive use.
	cmd := backendCommand(s.session.backend)
	// Execute via sandbox.Exec (one-shot, but with full context in prompt).
	var out strings.Builder
	exitCode, execErr := s.session.sandbox.Exec(
		ctx,
		cmd,
		strings.NewReader(prompt),
		&out, &out,
	)
	if execErr != nil {
		sendJSONRPCError(conn, id, -32000, execErr.Error())
		return
	}

	output := out.String()

	// Record the exchange in conversation history for the next call.
	s.appendExchange(instruction, output)

	_ = json.NewEncoder(conn).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  runResult{Output: output, ExitCode: exitCode},
		ID:      id,
	})
}

// sendJSONRPCError writes a JSON-RPC error response to conn.
func sendJSONRPCError(conn net.Conn, id interface{}, code int, message string) {
	_ = json.NewEncoder(conn).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		Error:   map[string]interface{}{"code": code, "message": message},
		ID:      id,
	})
}

// ---------------------------------------------------------------------------
// Engine validation
// ---------------------------------------------------------------------------

// ValidateOfflineEngine returns an error if the definition's engine is not
// "local".  Offline mode only supports the local engine; docker, cloudflare,
// and custom engines are rejected.
func ValidateOfflineEngine(def *sandbox.SandboxBundleMetadata) error {
	if def.Engine != "" && def.Engine != "local" {
		return fmt.Errorf("offline mode supports only 'local' engine, got %q", def.Engine)
	}
	return nil
}

// backendCommand returns the command arguments for a given backend name.
// Backends like claude need -p (non-interactive) flag when fed via stdin.
// If the backend is a bare name (not a path), it resolves from PATH using the
// same order the shell would (checks each PATH entry in order).
func backendCommand(backend string) []string {
	// Resolve bare names to absolute paths, checking PATH entries in order
	// so the first match wins, matching shell behavior.
	path := backend
	if !strings.Contains(backend, "/") {
		path = resolveFromPATH(backend)
	}

	switch backend {
	case "claude":
		return []string{path, "-p"}
	case "codex":
		return []string{path, "-p"}
	default:
		return []string{path}
	}
}

// resolveFromPATH searches each directory in PATH for the named executable
// and returns the first match. Unlike exec.LookPath, this doesn't cache or
// use Go's internal resolver — it directly checks the environment.
func resolveFromPATH(name string) string {
	pathEnv := os.Getenv("PATH")
	dirs := filepath.SplitList(pathEnv)

	// For "claude", prefer canonical install paths that are known to work
	// under sandbox-exec, regardless of PATH order.
	if name == "claude" {
		preferred := []string{
			filepath.Join(os.Getenv("HOME"), ".local", "bin", "claude"),
		}
		for _, p := range preferred {
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
				return p
			}
		}
	}

	for _, dir := range dirs {
		candidate := filepath.Join(dir, name)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
			return candidate
		}
	}
	return name
}

// Compile-time interface check.
var _ io.Writer = (*strings.Builder)(nil)
