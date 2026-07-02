// Binary mework-mcp is a stdio-based MCP server that exposes mework's sandbox
// lifecycle, session context, and notification capabilities as callable tools
// for an AI agent orchestrator. When MEWORK_MCP_ADDR is set, it runs in HTTP
// (Streamable HTTP) mode instead.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Build-time variables — injected via -ldflags (see .goreleaser.yml).
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	reg := NewToolRegistry()

	// Wire up handlers.
	var sandboxMgr SandboxManager
	runnerID := os.Getenv("MEWORK_RUNNER_ID")
	if runnerID != "" {
		sandboxMgr = NewRealSandboxManager()
		log.Printf("using hub-backed sandbox manager (runner %s)", runnerID)
	} else {
		sandboxMgr = NewLocalSandboxManager()
		log.Println("using local sandbox manager (no hub)")
	}
	sandboxH := NewSandboxHandler(sandboxMgr)
	notifyH := NewNotifyHandler(NewRealBusBroker())
	sessionH := NewSessionHandler()

	// Register sandbox lifecycle tools.
	reg.Register("spawn_sandbox", mcp.NewTool("spawn_sandbox",
		mcp.WithDescription("Spawn a child sandbox for delegated work"),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent identifier")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt for the child agent")),
		mcp.WithString("image", mcp.Description("Container image (default: ubuntu:22.04)")),
		mcp.WithNumber("timeout_minutes", mcp.Description("Max run time in minutes")),
		mcp.WithString("workspace_path", mcp.Description("Workspace path to mount")),
			mcp.WithString("parent_id", mcp.Description("Orchestrator session ID that owns this sandbox")),
	), sandboxH.SpawnSandbox)
	reg.Register("get_sandbox_status", mcp.NewTool("get_sandbox_status",
		mcp.WithDescription("Get status of a child sandbox"),
		mcp.WithString("sandbox_id", mcp.Required(), mcp.Description("Sandbox identifier")),
	), sandboxH.GetSandboxStatus)
	reg.Register("list_child_sandboxes", mcp.NewTool("list_child_sandboxes",
		mcp.WithDescription("List all active child sandboxes"),
			mcp.WithString("parent_id", mcp.Description("Filter: only children of this parent")),
			mcp.WithString("agent_name", mcp.Description("Filter: find sandbox by agent name")),
	), sandboxH.ListChildSandboxes)
	reg.Register("destroy_sandbox", mcp.NewTool("destroy_sandbox",
		mcp.WithDescription("Stop and destroy a child sandbox"),
		mcp.WithString("sandbox_id", mcp.Required(), mcp.Description("Sandbox identifier")),
	), sandboxH.DestroySandbox)
	reg.Register("wait_for_sandbox", mcp.NewTool("wait_for_sandbox",
		mcp.WithDescription("Wait for a child sandbox to complete"),
		mcp.WithString("sandbox_id", mcp.Required(), mcp.Description("Sandbox identifier")),
		mcp.WithNumber("timeout_seconds", mcp.Description("Max wait time in seconds")),
	), sandboxH.WaitForSandbox)
	reg.Register("send_to_sandbox", mcp.NewTool("send_to_sandbox",
		mcp.WithDescription("Send a message to a running sandbox's stdin"),
		mcp.WithString("sandbox_id", mcp.Required(), mcp.Description("Sandbox identifier")),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message content")),
	), sandboxH.SendToSandbox)

	// Register notification tools.
	reg.Register("notify_human", mcp.NewTool("notify_human",
		mcp.WithDescription("Send a message to the human through the session output"),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message content")),
		mcp.WithString("format", mcp.Description("Format: text (default) or markdown")),
	), notifyH.NotifyHuman)
	reg.Register("ask_human", mcp.NewTool("ask_human",
		mcp.WithDescription("Ask the human a question and wait for a response"),
		mcp.WithString("question", mcp.Required(), mcp.Description("Question to ask")),
		mcp.WithNumber("timeout_minutes", mcp.Description("Max wait time in minutes")),
	), notifyH.AskHuman)

	// Register session context tools.
	reg.Register("get_session_context", mcp.NewTool("get_session_context",
		mcp.WithDescription("Get the orchestrator's session identity and workspace info"),
	), sessionH.GetSessionContext)
	reg.Register("write_artifact", mcp.NewTool("write_artifact",
		mcp.WithDescription("Write content to the session workspace"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Relative path within workspace")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Content to write")),
		mcp.WithString("encoding", mcp.Description("Encoding: text (default) or base64")),
	), sessionH.WriteArtifact)

	mcpServer := server.NewMCPServer("mework-mcp", "1.0.0")
	mcpServer.AddTools(reg.ServerTools()...)

	// Mode selection: HTTP (MEWORK_MCP_ADDR set) or stdio (default).
	addr := os.Getenv("MEWORK_MCP_ADDR")
	if addr != "" {
		// HTTP mode (Streamable HTTP) — run as a standalone server.
		s := server.NewStreamableHTTPServer(mcpServer)
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		go func() {
			log.Printf("mework-mcp listening on %s/mcp", addr)
			if err := s.Start(addr); err != nil {
				log.Fatalf("mework-mcp: %v", err)
			}
		}()
		<-ctx.Done()
		_ = s.Shutdown(context.Background())
	} else {
		// Stdio mode — Claude Code spawns us as a subprocess via settings.json.
		s := server.NewStdioServer(mcpServer)
		if err := s.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
			log.Fatalf("mework-mcp: %v", err)
		}
	}
}
