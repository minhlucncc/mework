// Package core holds the canonical domain types shared across all mework
// components. These are stub definitions that downstream changes will fill in.
package core

import "time"

// Agent represents an AI coding agent that can be run in a sandbox.
type Agent struct {
	ID   string
	Kind string
	Name string
}

// Run is a single execution of an agent on a task.
type Run struct {
	ID      string
	AgentID string
	Spec    RunSpec
}

// Session represents a long-lived interaction between a user and an agent.
type Session struct {
	ID      string
	AgentID string
	UserID  string
}

// Grant is a signed permission allowing an agent to access a resource.
type Grant struct {
	ID       string
	Resource string
	Action   string
}

// Topic is a message-bus topic name.
type Topic struct {
	Name string
}

// Message is an event published on the message bus.
type Message struct {
	ID        string
	Topic     Topic
	Payload   []byte
	ContentType string
}

// RunSpec describes how to run an agent: which agent, which task, resource limits.
type RunSpec struct {
	AgentID   string
	Task      string
	SandboxID string
}

// Result is the output of a completed agent run.
type Result struct {
	RunID    string
	ExitCode int
	Output   string
	Error    string
}

// Workspace is a synced working directory for an agent run.
type Workspace struct {
	ID      string
	Path    string
	Spec    *WorkspaceSpec
	Session *Session
}

// WorkspaceMode indicates whether a mount is read-write or read-only.
type WorkspaceMode string

const (
	WorkspaceModeRW WorkspaceMode = "rw"
	WorkspaceModeRO WorkspaceMode = "ro"
)

// SyncMode controls when workspace changes are pushed to the remote object store.
type SyncMode string

const (
	SyncModeContinuous SyncMode = "continuous"
	SyncModeOnFlush    SyncMode = "on_flush"
	SyncModeManual     SyncMode = "manual"
)

// BaseKind is the type of base source for a workspace.
type BaseKind string

const (
	BaseKindGit     BaseKind = "git"
	BaseKindArchive BaseKind = "archive"
	BaseKindStore   BaseKind = "store"
)

// BaseSpec describes how to materialize the workspace base before the agent runs.
type BaseSpec struct {
	Kind BaseKind
	Ref  string // URL (git) or object-store ref (archive, store)
	Rev  string // git revision (branch, tag, commit); empty means default
}

// HookStage identifies a point in the workspace lifecycle where hooks run.
type HookStage string

const (
	HookStageInit     HookStage = "init"
	HookStagePreRun   HookStage = "pre_run"
	HookStagePostRun  HookStage = "post_run"
	HookStagePreSync  HookStage = "pre_sync"
	HookStagePostSync HookStage = "post_sync"
)

// HookResult captures the outcome of a lifecycle hook execution.
type HookResult struct {
	Stage    HookStage
	ExitCode int
	Output   string
	Error    string
}

// SyncResult captures the outcome of a sync operation.
type SyncResult struct {
	Pushed int
	Pulled int
	Failed int
}

// WorkspaceSpec describes how to create and manage a workspace for a session.
type WorkspaceSpec struct {
	MountPath    string
	RemotePrefix string
	Mode         WorkspaceMode
	Sync         SyncMode
	SharedRoots  []string
	Base         *BaseSpec
	Hooks        []Hook
}

// ObjectRef identifies an object in an object store (bucket + key).
type ObjectRef struct {
	Bucket string
	Key    string
}

// ObjectInfo is metadata about a stored object.
type ObjectInfo struct {
	Ref       ObjectRef
	Size      int64
	ETag      string
}

// Hook is a lifecycle hook (before/after run, before/after agent step).
type Hook struct {
	Name   string
	Script string
	Stage  HookStage
}

// SandboxCaps describes what a sandbox engine can do.
type SandboxCaps struct {
	MaxMemoryMB    int
	MaxDiskMB      int
	SupportsGPU    bool
	SupportsNet    bool
	IsIsolated     bool
}

// SessionID uniquely identifies a session.
type SessionID string

// AccountID uniquely identifies a user account.
type AccountID string

// TenantID uniquely identifies a tenant.
type TenantID string

// SessionStatus represents the lifecycle state of a session.
type SessionStatus string

const (
	SessionActive SessionStatus = "active"
	SessionIdle   SessionStatus = "idle"
	SessionClosed SessionStatus = "closed"
)

// SessionInfo is the management view of a live agent association.
type SessionInfo struct {
	ID      SessionID
	Tenant  TenantID
	Runner  string
	Agent   Agent
	Status  SessionStatus
	Owner   AccountID
	Created time.Time
}
