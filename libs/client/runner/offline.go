package runner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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

	// When non-empty, subsequent messages are forwarded directly to this
	// sandbox via the local MCP HTTP server (bypassing the orchestrator).
	joinedSandbox string
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

// SetPolicy attaches a message policy to the server. When set, every
// incoming "run" request is checked against the policy before execution.
func (s *OfflineServer) SetPolicy(p *policy.Policy) {
	s.policy = p
	s.rateLimiter = policy.NewRateLimiter()
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

// appendExchange adds one user->assistant exchange to the conversation history,
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

// ---------------------------------------------------------------------------
// MCP sandbox communication
// ---------------------------------------------------------------------------

// sandboxMCPURL is the local MCP HTTP server endpoint for sandbox operations.
const sandboxMCPURL = "http://localhost:18789/mcp"

// sendToSandboxViaMCP forwards a message to a running sandbox over the MCP
// HTTP API and returns the response.
func (s *OfflineServer) sendToSandboxViaMCP(sandboxID, message string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Initialize an MCP session to get a session ID.
	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"mework-daemon","version":"1.0"}}}`
	resp, err := client.Post(sandboxMCPURL, "application/json", strings.NewReader(initPayload))
	if err != nil {
		return "", fmt.Errorf("mcp connect: %w", err)
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	resp.Body.Close()
	if sessionID == "" {
		return "", fmt.Errorf("no MCP session id")
	}

	// Call send_to_sandbox tool.
	escaped := strings.ReplaceAll(message, `"`, `\"`)
	callPayload := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_to_sandbox","arguments":{"sandbox_id":"%s","message":"%s"}}}`,
		sandboxID, escaped,
	)
	req, _ := http.NewRequest("POST", sandboxMCPURL, strings.NewReader(callPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp2, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("mcp send: %w", err)
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)

	// Parse the tool response.
	var rpcResp struct {
		Result *struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return string(body), nil
	}
	if rpcResp.Error != nil {
		return "", fmt.Errorf("mcp error: %s", rpcResp.Error.Message)
	}
	if rpcResp.Result != nil && len(rpcResp.Result.Content) > 0 {
		return rpcResp.Result.Content[0].Text, nil
	}
	return string(body), nil
}

// ---------------------------------------------------------------------------
// Connection handling — JSON-RPC over Unix socket
// ---------------------------------------------------------------------------

// Start unlinks any stale socket at the path, begins listening, and accepts
// connections in a background goroutine.  It blocks until ctx is cancelled
// or a fatal error occurs.
func (s *OfflineServer) Start(ctx context.Context) error {
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket %s: %w", s.socketPath, err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("chmod %s: %w", s.socketPath, err)
	}
	s.listener = listener

	go s.acceptLoop(ctx)

	<-ctx.Done()
	_ = s.listener.Close()
	return ctx.Err()
}

// Close removes the socket file and marks the server as shut down.
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

// acceptLoop accepts connections and dispatches each in its own goroutine.
func (s *OfflineServer) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConnection(ctx, conn)
	}
}

// ---------------------------------------------------------------------------
// JSON-RPC types
// ---------------------------------------------------------------------------

type jsonRPCRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     interface{}     `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type runParams struct {
	Instruction string `json:"instruction"`
	Sender      string `json:"sender,omitempty"`
}

type runResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// handleConnection reads one JSON-RPC request and dispatches it.
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

// findSandboxIDByAgent resolves an agent name to a sandbox ID by querying
// the local MCP server's list_child_sandboxes with the agent_name filter.
func (s *OfflineServer) findSandboxIDByAgent(agentName string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"mework-daemon","version":"1.0"}}}`
	resp, err := client.Post(sandboxMCPURL, "application/json", strings.NewReader(initPayload))
	if err != nil {
		return "", fmt.Errorf("mcp connect: %w", err)
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	resp.Body.Close()
	if sessionID == "" {
		return "", fmt.Errorf("no MCP session id")
	}

	callPayload := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_child_sandboxes","arguments":{"agent_name":"%s"}}}`,
		agentName,
	)
	req, _ := http.NewRequest("POST", sandboxMCPURL, strings.NewReader(callPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp2, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("mcp list: %w", err)
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)

	var rpcResp struct {
		Result *struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return "", fmt.Errorf("parse mcp response: %w", err)
	}
	if rpcResp.Result == nil || len(rpcResp.Result.Content) == 0 {
		return "", fmt.Errorf("no MCP response")
	}

	var listResult struct {
		Children []struct {
			SandboxID string `json:"sandbox_id"`
			AgentID   string `json:"agent_id"`
		} `json:"children"`
	}
	if err := json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &listResult); err != nil {
		return "", fmt.Errorf("parse list result: %w", err)
	}
	if len(listResult.Children) == 0 {
		return "", fmt.Errorf("no sandbox found with agent name %q", agentName)
	}
	return listResult.Children[0].SandboxID, nil
}

// handleRun routes the instruction: to the orchestrator by default, or
// directly to a joined sandbox when /join was used.
func (s *OfflineServer) handleRun(ctx context.Context, conn net.Conn, id interface{}, instruction string) {
	trimmed := strings.TrimSpace(instruction)

	// /join <sandbox_id_or_name> — switch to direct sandbox mode.
	// If the argument is a name (no dashes or only short), resolve it to an ID.
	if strings.HasPrefix(trimmed, "/join ") {
		parts := strings.Fields(trimmed)
		if len(parts) >= 2 {
			joinTarget := parts[1]
			var msg string
			// Try to resolve name to sandbox ID via MCP lookup.
			if sid, err := s.findSandboxIDByAgent(joinTarget); err == nil && sid != "" {
				s.joinedSandbox = sid
				msg = fmt.Sprintf("Joined sandbox %s (%s). Messages go directly there. Use /leave to return.", joinTarget, sid)
			} else {
				s.joinedSandbox = joinTarget
				msg = fmt.Sprintf("Joined sandbox %s. Messages go directly there. Use /leave to return.", s.joinedSandbox)
			}
			_ = json.NewEncoder(conn).Encode(jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  runResult{Output: msg, ExitCode: 0},
				ID:      id,
			})
			return
		}
	}

	// /leave — return to orchestrator mode.
	if trimmed == "/leave" {
		s.joinedSandbox = ""
		_ = json.NewEncoder(conn).Encode(jsonRPCResponse{
			JSONRPC: "2.0",
			Result:  runResult{Output: "Left sandbox mode. Back to orchestrator.", ExitCode: 0},
			ID:      id,
		})
		return
	}

	// Check for unrecognized commands (user typos).
	if strings.HasPrefix(trimmed, "/") && trimmed != "/leave" && !strings.HasPrefix(trimmed, "/join ") {
		// Not a daemon command — pass through to orchestrator or sandbox.
		// Just continue to the normal handling below.
	}

	// Validate known commands — notify user on typos.
	if strings.HasPrefix(trimmed, "/") {
		cmdName := trimmed
		if idx := strings.IndexByte(trimmed, ' '); idx > 0 {
			cmdName = trimmed[:idx]
		}
		known := map[string]bool{
			"/spawn": true, "/sessions": true, "/status": true, "/stop": true,
			"/join": true, "/leave": true, "/exit": true, "/quit": true,
			"/preview": true, "/preview-stop": true, "/opsx": true,
		}
		if !known[cmdName] {
			_ = json.NewEncoder(conn).Encode(jsonRPCResponse{
				JSONRPC: "2.0",
				Result:  runResult{Output: "Unknown command: " + cmdName + "\nValid: /spawn, /sessions, /status, /stop, /join, /leave, /exit", ExitCode: 0},
				ID:      id,
			})
			return
		}
	}

	// If joined to a sandbox, forward the message directly.
	if s.joinedSandbox != "" {
		result, err := s.sendToSandboxViaMCP(s.joinedSandbox, instruction)
		if err != nil {
			sendJSONRPCError(conn, id, -32000, "sandbox: "+err.Error())
			return
		}
		var outText string
		var data struct {
			Result *struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(result), &data) == nil && data.Result != nil && len(data.Result.Content) > 0 {
			outText = data.Result.Content[0].Text
		} else {
			outText = result
		}
		_ = json.NewEncoder(conn).Encode(jsonRPCResponse{
			JSONRPC: "2.0",
			Result:  runResult{Output: outText, ExitCode: 0},
			ID:      id,
		})
		return
	}

	// Normal orchestrator mode.
	prompt := s.buildPrompt(instruction)
	cmd := backendCommand(s.session.backend)
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
func backendCommand(backend string) []string {
	path := backend
	if !strings.Contains(backend, "/") {
		path = resolveFromPATH(backend)
	}

	switch backend {
	case "claude":
		return []string{path, "-p", "--dangerously-skip-permissions"}
	case "codex":
		return []string{path, "-p"}
	default:
		return []string{path}
	}
}

// resolveFromPATH searches each directory in PATH for the named executable.
func resolveFromPATH(name string) string {
	pathEnv := os.Getenv("PATH")
	dirs := filepath.SplitList(pathEnv)

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
