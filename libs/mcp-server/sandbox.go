package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// ChildSandbox holds the metadata for a tracked child sandbox.
type ChildSandbox struct {
	AgentID   string    `json:"agent_id"`
	ParentID  string    `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SandboxManager defines the interface for sandbox lifecycle operations.
type SandboxManager interface {
	Start(ctx context.Context, agentID, prompt, image string) (sandboxID string, err error)
	Stop(ctx context.Context, sandboxID string) error
	Destroy(ctx context.Context, sandboxID string) error
	Status(ctx context.Context, sandboxID string) (status, result string, err error)
	List(ctx context.Context) ([]string, error)
	Wait(ctx context.Context, sandboxID string, timeout time.Duration) (status, result string, err error)
	Send(ctx context.Context, sandboxID, message string) error
}

// SandboxHandler implements MCP tool handlers for sandbox lifecycle tools.
// All sandboxes are automatically tagged with the caller's parent ID (from
// MEWORK_SESSION_ID env var), and list operations are scoped to that parent.
type SandboxHandler struct {
	mu              sync.Mutex
	manager         SandboxManager
	infos           map[string]ChildSandbox
	defaultParentID string
}

// NewSandboxHandler creates a new SandboxHandler backed by the given manager.
func NewSandboxHandler(manager SandboxManager) *SandboxHandler {
	parentID := os.Getenv("MEWORK_SESSION_ID")
	if parentID == "" {
		parentID = "default"
	}
	return &SandboxHandler{
		manager:         manager,
		infos:           make(map[string]ChildSandbox),
		defaultParentID: parentID,
	}
}

// callerParent returns the effective parent ID for the calling session.
// It reads the MCP session identity from environment; when the orchestrator
// daemon starts the MCP server, MEWORK_SESSION_ID is set to the agent name
// (e.g. "mework-dev") so all spawned sandboxes are automatically scoped.
func (h *SandboxHandler) callerParent() string {
	// Allow override via parent_id arg, but default to env-based identity.
	if pid := os.Getenv("MEWORK_SESSION_ID"); pid != "" {
		return pid
	}
	return h.defaultParentID
}

// SpawnSandbox handles the spawn_sandbox tool.
// The sandbox is automatically tagged with the caller's parent ID so that
// list_child_sandboxes can enforce per-orchestrator scoping.
func (h *SandboxHandler) SpawnSandbox(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	agentID, _ := args["agent_id"].(string)
	prompt, _ := args["prompt"].(string)
	image, _ := args["image"].(string)

	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	sandboxID, err := h.manager.Start(ctx, agentID, prompt, image)
	if err != nil {
		return nil, fmt.Errorf("start sandbox: %w", err)
	}

	// Auto-tag with the caller's parent ID (code-enforced, not prompt-based).
	parentID := h.callerParent()

	h.mu.Lock()
	h.infos[sandboxID] = ChildSandbox{
		AgentID:   agentID,
		ParentID:  parentID,
		CreatedAt: time.Now(),
	}
	h.mu.Unlock()

	return map[string]string{"sandbox_id": sandboxID}, nil
}

// GetSandboxStatus handles the get_sandbox_status tool.
func (h *SandboxHandler) GetSandboxStatus(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sandboxID, _ := args["sandbox_id"].(string)
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}

	status, result, err := h.manager.Status(ctx, sandboxID)
	if err != nil {
		return nil, err
	}

	resp := map[string]interface{}{
		"status": status,
		"result": result,
	}

	h.mu.Lock()
	if info, ok := h.infos[sandboxID]; ok {
		resp["created_at"] = info.CreatedAt.Format(time.RFC3339)
		resp["agent_id"] = info.AgentID
	}
	h.mu.Unlock()

	return resp, nil
}

// ListChildSandboxes handles the list_child_sandboxes tool.
// By default, only returns sandboxes owned by the caller's session (parent_id).
// The agent_name filter finds a sandbox by its agent_id (for /join by name).
func (h *SandboxHandler) ListChildSandboxes(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	agentFilter, _ := args["agent_name"].(string)

	ids, err := h.manager.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}

	// Auto-scope: only show sandboxes owned by this caller (code-enforced).
	callerID := h.callerParent()

	children := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		status, result, err := h.manager.Status(ctx, id)
		if err != nil {
			continue
		}

		h.mu.Lock()
		info, hasInfo := h.infos[id]
		h.mu.Unlock()

		// Code-enforce parent_id scoping: skip sandboxes not owned by caller.
		if !hasInfo || info.ParentID != callerID {
			continue
		}
		// Apply agent_name filter (for /join by name).
		if agentFilter != "" && info.AgentID != agentFilter {
			continue
		}

		child := map[string]interface{}{
			"sandbox_id": id,
			"status":     status,
			"result":     result,
		}
		if hasInfo {
			child["agent_id"] = info.AgentID
			child["parent_id"] = info.ParentID
			child["created_at"] = info.CreatedAt.Format(time.RFC3339)
		}

		children = append(children, child)
	}

	return map[string]interface{}{"children": children}, nil
}

// DestroySandbox handles the destroy_sandbox tool.
func (h *SandboxHandler) DestroySandbox(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sandboxID, _ := args["sandbox_id"].(string)
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}

	_, _, err := h.manager.Status(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox not found: %s", sandboxID)
	}

	if err := h.manager.Stop(ctx, sandboxID); err != nil {
		return nil, fmt.Errorf("stop sandbox: %w", err)
	}

	if err := h.manager.Destroy(ctx, sandboxID); err != nil {
		return nil, fmt.Errorf("destroy sandbox: %w", err)
	}

	h.mu.Lock()
	delete(h.infos, sandboxID)
	h.mu.Unlock()

	return map[string]string{"status": "destroyed"}, nil
}

// SendToSandbox handles the send_to_sandbox tool.
func (h *SandboxHandler) SendToSandbox(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sandboxID, _ := args["sandbox_id"].(string)
	message, _ := args["message"].(string)
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}

	// Verify ownership before sending.
	h.mu.Lock()
	info, hasInfo := h.infos[sandboxID]
	h.mu.Unlock()
	if hasInfo && info.ParentID != "" && info.ParentID != h.callerParent() {
		return nil, fmt.Errorf("sandbox %s belongs to another session", sandboxID)
	}

	if err := h.manager.Send(ctx, sandboxID, message); err != nil {
		return nil, fmt.Errorf("send to sandbox %s: %w", sandboxID, err)
	}
	return map[string]string{"status": "sent"}, nil
}

// WaitForSandbox handles the wait_for_sandbox tool.
func (h *SandboxHandler) WaitForSandbox(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sandboxID, _ := args["sandbox_id"].(string)
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}

	var timeout time.Duration
	if timeoutSeconds, ok := args["timeout_seconds"].(float64); ok && timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds * float64(time.Second))
	} else {
		timeout = 300 * time.Second
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	status, result, err := h.manager.Wait(waitCtx, sandboxID, timeout)
	if err != nil {
		if waitCtx.Err() != nil {
			return nil, fmt.Errorf("timeout waiting for sandbox %s", sandboxID)
		}
		return nil, err
	}

	return map[string]interface{}{
		"status": status,
		"result": result,
	}, nil
}
