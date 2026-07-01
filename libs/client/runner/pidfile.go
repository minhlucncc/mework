// Pidfile management for the offline-stack orchestrator.
//
// Unit 02 of c0047-mezon-offline-mode. The orchestrator writes the
// `~/.mework/runtime/offline-pids.json` file atomically with O_EXCL so two
// `mework daemon start --offline` invocations cannot race. The file records
// the workspace root, the start timestamp, and a row per spawned child
// (server, worker) with its PID, optional listen port, and log path.
//
// The Pidfile / Meta / PidfileChild / ErrAlreadyRunning types are declared in
// offline_stack_test.go (the unit's contract). This file adds the
// behavioural methods on those types.
package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

// pidfileMu serialises writers inside a single daemon process. The
// cross-process guarantee comes from O_EXCL on os.OpenFile below; the
// mutex prevents two goroutines in the same process from racing past the
// existence check before either has finished writing.
var pidfileMu sync.Mutex

// Write atomically creates the pidfile with the given meta. If a file
// already exists at the path, Write returns ErrAlreadyRunning. The file is
// written with mode 0600 per the auth-and-secrets invariants in CLAUDE.md.
//
// Atomicity comes from os.OpenFile(O_CREATE|O_EXCL): the kernel refuses to
// create the file if it already exists, so two concurrent invocations on
// different machines (or two processes on the same machine) race in the
// kernel rather than in userspace.
func (p *Pidfile) Write(meta Meta) error {
	if p == nil || p.Path == "" {
		return errors.New("pidfile: empty path")
	}

	pidfileMu.Lock()
	defer pidfileMu.Unlock()

	f, err := os.OpenFile(p.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrAlreadyRunning
		}
		return fmt.Errorf("pidfile: open %s: %w", p.Path, err)
	}

	enc := json.NewEncoder(f)
	if err := enc.Encode(meta); err != nil {
		_ = f.Close()
		_ = os.Remove(p.Path)
		return fmt.Errorf("pidfile: encode: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("pidfile: close: %w", err)
	}
	// Belt-and-braces: ensure perms are 0600 even if a pre-existing file
	// survived the OpenFile call (shouldn't, but be defensive).
	if err := os.Chmod(p.Path, 0o600); err != nil {
		return fmt.Errorf("pidfile: chmod: %w", err)
	}
	return nil
}

// Read parses the pidfile JSON. Returns an error if the file is missing or
// malformed. Empty path or nil receiver both error.
func (p *Pidfile) Read() (Meta, error) {
	if p == nil || p.Path == "" {
		return Meta{}, errors.New("pidfile: empty path")
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return Meta{}, fmt.Errorf("pidfile: read %s: %w", p.Path, err)
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, fmt.Errorf("pidfile: decode: %w", err)
	}
	return meta, nil
}

// Remove deletes the pidfile. A missing file is not an error — Remove is
// idempotent so it can be called from defer and from the signal handler.
func (p *Pidfile) Remove() error {
	if p == nil || p.Path == "" {
		return nil
	}
	if err := os.Remove(p.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("pidfile: remove %s: %w", p.Path, err)
	}
	return nil
}