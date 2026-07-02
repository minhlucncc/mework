---
tags: [sandbox, security]
inject: always
---

## Sandbox security awareness

This project has a pluggable sandbox system. The sandbox.yaml defines the isolation level.

### Current sandbox configuration

- **Local engine**: auto-detects `sandbox-exec` on macOS → wraps in no-write profile
- **Docker engine**: hardened with --cap-drop=ALL, --read-only, --network none, --user nobody:nogroup
- **Cloudflare engine**: remote API execution (no filesystem)

### Engineering rules when modifying sandbox code

1. NEVER disable or bypass the safety controls (cap-drop, read-only, no-new-privileges)
2. NEVER run Docker with --privileged or mount the Docker socket
3. NEVER introduce a mechanism that places prompt content on the command line (stdin only)
4. If adding a new engine, ensure it reports accurate `Caps()` including `IsIsolated`
5. Security audit tests in `libs/sandbox/engine/*/security_audit_test.go` must pass
6. The `injector_audit_test.go` must confirm secrets write Value, not Source

### macOS sandbox-exec notes

- Uses built-in `no-write` profile (custom profiles with `(deny default)` fail on macOS 15+)
- Prevents writes to system paths and outside the workdir
- Does NOT restrict reads or network access
- If stronger isolation is needed, use the Docker engine instead

### Docker engine hardening notes

- `--cap-drop=ALL` with selective `--cap-add` for CHOWN, DAC_OVERRIDE, SETUID, SETGID, FOWNER
- `--security-opt no-new-privileges:true`
- `--read-only` rootfs with `--tmpfs` mounts for /tmp, /var/tmp, /run
- `--user nobody:nogroup`
- `--network none` by default (opt-in via MEWORK_NETWORK=1)
- Default resource limits: 512m memory, 0.5 CPU, 100 pids limit
