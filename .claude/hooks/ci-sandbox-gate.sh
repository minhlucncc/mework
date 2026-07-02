#!/bin/bash
# CI sandbox gate — verifies all sandbox definitions pass security checks.
# Called by CI pipeline before merging. Exit non-zero blocks the merge.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
FAILED=0

echo "=== Sandbox security CI gate ==="

for yaml in $(find "${ROOT}/libs/sandbox/definitions" -name "sandbox.yaml" 2>/dev/null || true); do
  echo "Checking: $yaml"

  # 1. Must not use engine: local in production definitions
  ENGINE=$(grep -E "^engine:" "$yaml" | awk '{print $2}' | tr -d ' ')
  if [ "$ENGINE" = "local" ] && echo "$yaml" | grep -qv "local-claude"; then
    echo "  FAIL: uses local engine (no isolation)"
    FAILED=1
  fi

  # 2. Must have image pinned (not :latest) for container engines
  if [ "$ENGINE" = "docker" ]; then
    IMAGE=$(grep -E "^image:" "$yaml" | awk '{print $2}' | tr -d ' ')
    if echo "$IMAGE" | grep -qE ":latest$"; then
      echo "  FAIL: uses :latest tag (must pin a specific version)"
      FAILED=1
    fi
    # Check resource limits
    if ! grep -q "resourceLimits:" "$yaml"; then
      echo "  FAIL: missing resourceLimits (required for docker engine)"
      FAILED=1
    fi
  fi

  echo "  OK"
done

# Run security audit tests
echo ""
echo "Running security audit tests..."
if go test -count=1 -run "TestAudit" ./libs/shared/grant/ ./libs/sandbox/engine/local/ ./libs/sandbox/runtime/ 2>&1; then
  echo "Security audit tests: PASSED"
else
  echo "Security audit tests: FAILED"
  FAILED=1
fi

if [ "$FAILED" -eq 1 ]; then
  echo ""
  echo "Sandbox security CI gate: FAILED"
  exit 1
fi
echo ""
echo "Sandbox security CI gate: PASSED"
