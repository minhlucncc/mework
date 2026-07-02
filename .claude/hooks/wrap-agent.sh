#!/bin/bash
# Wrap-agent hook — wraps the agent subprocess in a sandbox.
# This is an executable hook that Claude Code calls before running
# each agent turn. It wraps the command in sandbox-exec (macOS) or
# Docker to provide process isolation.
#
# Env vars (set by Claude Code or the invocation context):
#   CLAUDE_HOOK_STAGE    - "pre" or "post"
#   CLAUDE_WORKSPACE_ROOT - project root
#   CLAUDE_COMMAND       - the command being run
#
# To enable sandboxing, set MEWORK_SANDBOX_WRAP=1 in your environment:
#   MEWORK_SANDBOX_WRAP=1 claude ...
set -euo pipefail

if [ "${MEWORK_SANDBOX_WRAP:-0}" != "1" ]; then
  # Sandbox wrapping disabled — pass through
  exec "$@"
fi

OS="$(uname)"

case "$OS" in
  Darwin)
    # Use sandbox-exec with no-write profile to prevent filesystem modifications
    if command -v sandbox-exec &>/dev/null; then
      echo "[sandbox-wrap] wrapping in sandbox-exec (no-write)" >&2
      exec sandbox-exec -n no-write -- "$@"
    fi
    echo "[sandbox-wrap] WARNING: sandbox-exec not available, running without sandbox" >&2
    exec "$@"
    ;;
  Linux)
    # Use Docker if available
    if command -v docker &>/dev/null; then
      echo "[sandbox-wrap] wrapping in Docker container" >&2
      IMAGE="${MEWORK_SANDBOX_IMAGE:-ubuntu:22.04}"
      exec docker run --rm -i --network none --read-only \
        --cap-drop=ALL --cap-add=CHOWN,DAC_OVERRIDE,SETUID,SETGID \
        --user nobody:nogroup \
        --workdir /work \
        --tmpfs /tmp:rw,noexec,nosuid,size=100m \
        --memory 512m --cpus 0.5 \
        -v "$(pwd):/work:ro" \
        "$IMAGE" \
        "$@"
    fi
    echo "[sandbox-wrap] WARNING: Docker not available, running without sandbox" >&2
    exec "$@"
    ;;
  *)
    echo "[sandbox-wrap] WARNING: unsupported OS $OS, running without sandbox" >&2
    exec "$@"
    ;;
esac
