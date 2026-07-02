#!/bin/bash
# Pre-dispatch hook — validates sandbox config before agent dispatch.
# Called by Claude Code before each agent run.
# Exit non-zero to block the dispatch.
set -euo pipefail

ROOT="${CLAUDE_WORKSPACE_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
SANDBOX_YAML="${ROOT}/.claude/sandbox.yaml"

# If no sandbox.yaml, warn and allow (dev mode)
if [ ! -f "$SANDBOX_YAML" ]; then
  echo "[sandbox-hook] WARNING: no sandbox.yaml found. Agent runs with default engine (no isolation)."
  exit 0
fi

ENGINE=$(grep -E "^engine:" "$SANDBOX_YAML" | awk '{print $2}' | tr -d ' ')
case "$ENGINE" in
  docker)
    echo "[sandbox-hook] Using Docker engine — container isolation active."
    ;;
  sandbox-exec|macos)
    if [ "$(uname)" != "Darwin" ]; then
      echo "[sandbox-hook] ERROR: engine '$ENGINE' requires macOS but OS is $(uname)" >&2
      exit 1
    fi
    echo "[sandbox-hook] Using sandbox-exec — filesystem write isolation active."
    ;;
  local|"")
    echo "[sandbox-hook] WARNING: engine is 'local' — NO host isolation."
    echo "[sandbox-hook] Set engine: docker in sandbox.yaml for container isolation."
    ;;
  *)
    echo "[sandbox-hook] ERROR: unknown engine '$ENGINE' in sandbox.yaml" >&2
    exit 1
    ;;
esac

# Check that policy is set for container engines
if [ "$ENGINE" = "docker" ] && ! grep -qE "^policy:" "$SANDBOX_YAML"; then
  echo "[sandbox-hook] WARNING: Docker engine without policy block — no message-level access control."
fi
