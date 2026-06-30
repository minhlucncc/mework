package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// RealSandboxManager implements SandboxManager by connecting to the mework
// daemon's sandbox API. For dev/standalone mode it returns descriptive
// "not connected" errors — the daemon connection is configured via
// environment variables.
type RealSandboxManager struct{}

// NewRealSandboxManager creates a RealSandboxManager.
// In production it reads the daemon address from MEWORK_DAEMON_ADDR.
func NewRealSandboxManager() *RealSandboxManager {
	return &RealSandboxManager{}
}

func (m *RealSandboxManager) Start(ctx context.Context, agentID, prompt, image string) (string, error) {
	addr := os.Getenv("MEWORK_DAEMON_ADDR")
	if addr == "" {
		return "", fmt.Errorf("mework-mcp: MEWORK_DAEMON_ADDR not set — sandbox operations require a running daemon")
	}
	log.Printf("RealSandboxManager: Start(agentID=%s, image=%s) via daemon at %s", agentID, image, addr)
	return "", fmt.Errorf("RealSandboxManager.Start: not fully implemented — daemon sandbox API TBD")
}

func (m *RealSandboxManager) Stop(ctx context.Context, sandboxID string) error {
	return fmt.Errorf("RealSandboxManager.Stop: not fully implemented")
}

func (m *RealSandboxManager) Destroy(ctx context.Context, sandboxID string) error {
	return fmt.Errorf("RealSandboxManager.Destroy: not fully implemented")
}

func (m *RealSandboxManager) Status(ctx context.Context, sandboxID string) (string, string, error) {
	return "", "", fmt.Errorf("RealSandboxManager.Status: not fully implemented")
}

func (m *RealSandboxManager) List(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("RealSandboxManager.List: not fully implemented")
}

func (m *RealSandboxManager) Wait(ctx context.Context, sandboxID string, timeout time.Duration) (string, string, error) {
	return "", "", fmt.Errorf("RealSandboxManager.Wait: not fully implemented")
}

// RealBusBroker implements BusBroker by connecting to the mework daemon's
// message bus. For dev/standalone mode it returns descriptive errors.
type RealBusBroker struct{}

// NewRealBusBroker creates a RealBusBroker.
func NewRealBusBroker() *RealBusBroker {
	return &RealBusBroker{}
}

func (b *RealBusBroker) Publish(ctx context.Context, topic string, payload []byte) error {
	addr := os.Getenv("MEWORK_DAEMON_ADDR")
	if addr == "" {
		return fmt.Errorf("mework-mcp: MEWORK_DAEMON_ADDR not set — bus operations require a running daemon")
	}
	log.Printf("RealBusBroker: Publish(topic=%s, %d bytes) via daemon at %s", topic, len(payload), addr)
	return fmt.Errorf("RealBusBroker.Publish: not fully implemented")
}

func (b *RealBusBroker) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	addr := os.Getenv("MEWORK_DAEMON_ADDR")
	if addr == "" {
		return nil, fmt.Errorf("mework-mcp: MEWORK_DAEMON_ADDR not set — bus operations require a running daemon")
	}
	log.Printf("RealBusBroker: Subscribe(topic=%s) via daemon at %s", topic, addr)
	return nil, fmt.Errorf("RealBusBroker.Subscribe: not fully implemented")
}
