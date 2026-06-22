## ADDED Requirements

### Requirement: Complete sandbox runtime surface

The sandbox runtime SHALL provide a usable out-of-process runner binary that selects an engine
and drives the full sandbox lifecycle (start, exec a turn over stdin, stream output, stop,
destroy) for a given definition and workspace. Every registered engine SHALL either fully
implement the lifecycle contract or return a **typed "unsupported"** error for a capability it
genuinely cannot provide (and advertise that via its capabilities) — no method may be a silent
no-op. The one-agent-per-sandbox and stdin-not-argv invariants hold throughout.

#### Scenario: Sandbox runner drives a session

- **WHEN** the sandbox runner is started for a definition + workspace with the local engine
- **THEN** it starts the sandbox, runs a turn delivered over stdin, streams output, and stops
  the sandbox

#### Scenario: Unsupported engine capability is explicit

- **WHEN** an engine is asked for a lifecycle capability it cannot provide
- **THEN** it returns a typed unsupported error (advertised via its capabilities) rather than
  silently doing nothing
