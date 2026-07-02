package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"mework/libs/sandbox/agent"
	"mework/libs/sandbox/runtime"
	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// sandboxState tracks a running sandbox and its output.
type sandboxState struct {
	sandbox ports.Sandbox
	status  string
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	done    chan struct{}
	err     error
	cmd     []string
	task    string
}

// LocalSandboxManager implements SandboxManager using the local sandbox engine
// directly, without needing a hub server or runner enrollment.
type LocalSandboxManager struct {
	mu   sync.Mutex
	mgr  *runtime.Manager
	jobs map[string]*sandboxState
}

// NewLocalSandboxManager creates a sandbox manager that spawns sandboxes locally.
func NewLocalSandboxManager() *LocalSandboxManager {
	mgr, err := runtime.NewManagerFor("local")
	if err != nil {
		log.Fatalf("local sandbox manager: %v", err)
	}
	return &LocalSandboxManager{
		mgr:  mgr,
		jobs: make(map[string]*sandboxState),
	}
}

func (m *LocalSandboxManager) Start(ctx context.Context, agentID, prompt, image string) (string, error) {
	workDir := os.Getenv("MEWORK_WORKSPACE")
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	if workDir == "" {
		workDir = os.TempDir()
	}

	sandboxID := agentID + "-" + fmt.Sprintf("%d", time.Now().UnixMilli())
	spec := core.RunSpec{
		SandboxID:   sandboxID,
		AgentID:     agentID,
		Task:        prompt,
		BackendName: "sh",
		Workspace:   core.Workspace{Path: workDir},
	}

	sb, err := m.mgr.Start(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("start: %w", err)
	}

	state := &sandboxState{
		sandbox: sb,
		status:  "running",
		done:    make(chan struct{}),
		task:    prompt,
	}

	m.mu.Lock()
	m.jobs[sandboxID] = state
	m.mu.Unlock()

	go func() {
		defer close(state.done)

		// Empty prompt = idle sandbox (long-running sleep, ready for send_to_sandbox).
		if strings.TrimSpace(prompt) == "" {
			state.cmd = []string{"/bin/sleep", "infinity"}
			_, _ = sb.Exec(context.Background(), state.cmd, nil, &state.stdout, &state.stderr)
			state.status = "done"
			return
		}

		execCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Try as shell command first. On failure, fall back to claude -p.
		state.cmd = []string{"/bin/sh", "-c", prompt}
		exitCode, execErr := sb.Exec(execCtx, state.cmd, nil, &state.stdout, &state.stderr)

		if execErr != nil || exitCode != 0 {
			// Shell failed — try Claude backend for NL prompts.
			backend, ok := agent.Detect([]string{"claude"})
			if ok {
				state.stdout.Reset()
				state.stderr.Reset()
				state.cmd = []string{backend.Path, "-p", "--dangerously-skip-permissions"}
				exitCode, execErr = sb.Exec(execCtx, state.cmd, strings.NewReader(prompt), &state.stdout, &state.stderr)
			}
		}

		if execErr != nil {
			state.err = execErr
			state.status = "failed"
			return
		}
		if exitCode != 0 {
			state.status = "failed"
			state.err = fmt.Errorf("exit code %d", exitCode)
			return
		}
		state.status = "done"
	}()

	return sandboxID, nil
}

func (m *LocalSandboxManager) Stop(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	state, ok := m.jobs[sandboxID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("sandbox %s not found", sandboxID)
	}
	return m.mgr.Stop(ctx, state.sandbox.ID())
}

func (m *LocalSandboxManager) Destroy(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	state, ok := m.jobs[sandboxID]
	if ok {
		delete(m.jobs, sandboxID)
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return m.mgr.Destroy(ctx, state.sandbox.ID())
}

func (m *LocalSandboxManager) Status(ctx context.Context, sandboxID string) (string, string, error) {
	m.mu.Lock()
	state, ok := m.jobs[sandboxID]
	m.mu.Unlock()
	if !ok {
		return "", "", fmt.Errorf("sandbox %s not found", sandboxID)
	}

	select {
	case <-state.done:
		if state.err != nil {
			return state.status, state.stderr.String(), nil
		}
		return state.status, state.stdout.String(), nil
	default:
		return "running", "", nil
	}
}

func (m *LocalSandboxManager) List(ctx context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.jobs))
	for id := range m.jobs {
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *LocalSandboxManager) Wait(ctx context.Context, sandboxID string, timeout time.Duration) (string, string, error) {
	m.mu.Lock()
	state, ok := m.jobs[sandboxID]
	m.mu.Unlock()
	if !ok {
		return "", "", fmt.Errorf("sandbox %s not found", sandboxID)
	}

	select {
	case <-state.done:
		if state.err != nil {
			return state.status, state.stderr.String(), nil
		}
		return state.status, state.stdout.String(), nil
	case <-time.After(timeout):
		return state.status, "", fmt.Errorf("timeout waiting for sandbox %s", sandboxID)
	case <-ctx.Done():
		return state.status, "", ctx.Err()
	}
}

func (m *LocalSandboxManager) Send(ctx context.Context, sandboxID, message string) error {
	m.mu.Lock()
	state, ok := m.jobs[sandboxID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("sandbox %s not found", sandboxID)
	}
	// For stdin-based communication, append to the task.
	state.task += "\n" + message
	state.stdout.WriteString("\n[user]: " + message + "\n")
	return nil
}
