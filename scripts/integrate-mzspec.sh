#!/usr/bin/env bash
# integrate-mzspec.sh — Integrate mework's orchestrator-worker model into an
# existing mzspec project. Non-destructive: adds files alongside existing ones,
# merges JSON config, appends to CLAUDE.md.
#
# Usage:
#   bash scripts/integrate-mzspec.sh --dest /path/to/project
#
# Adds:
#   .claude/skills/{session-manager,planner,communicator}/  (if missing)
#   .claude/commands/{sessions,spawn,status,stop}.md        (if missing)
#   mework-mcp + gh mcp to .claude/settings.json            (merged)
#   Orchestrator delegation rules to CLAUDE.md               (appended)
#   .mework/templates/worker-*.prompt.md                    (always)
#   mzspec.config.json                                       (if missing)

set -euo pipefail

MWDIR="$(cd "$(dirname "$0")/.." && pwd)"
DEST="${1:-$(pwd)}"
FORCE=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dest) DEST="${2:?}"; shift 2 ;;
    --force) FORCE=1; shift ;;
    --help) echo "Usage: integrate-mzspec.sh [--dest <path>] [--force]"; exit 0 ;;
    *) DEST="$1"; shift ;;
  esac
done

DEST="$(cd "$DEST" && pwd)"
TEMPLATE="$MWDIR/templates/mzspec"

echo "=== Integrating mework into mzspec project at: $DEST"
added=0; skipped=0

add_file() {
  local src="$1" dst="$2"
  mkdir -p "$(dirname "$dst")"
  if [ -e "$dst" ] && [ "$FORCE" -eq 0 ]; then
    echo "  SKIP $dst (exists, use --force to overwrite)"
    skipped=$((skipped + 1))
  else
    cp "$src" "$dst"
    echo "  ADD  $dst"
    added=$((added + 1))
  fi
}

merge_json() {
  local src="$1" dst="$2"
  mkdir -p "$(dirname "$dst")"
  if [ ! -e "$dst" ]; then
    cp "$src" "$dst"
    echo "  ADD  $dst (new)"
    added=$((added + 1))
    return
  fi
  # Merge "mcpServers" from src into dst using python3
  python3 -c "
import json, sys
with open('$src') as f: src = json.load(f)
with open('$dst') as f: dst = json.load(f)
src_servers = src.get('mcpServers', {})
dst_servers = dst.get('mcpServers', {})
for k, v in src_servers.items():
    if k not in dst_servers:
        dst_servers[k] = v
        print(f'  ADD  MCP server: {k}')
dst['mcpServers'] = dst_servers
with open('$dst', 'w') as f: json.dump(dst, f, indent=2)
" 2>&1
}

append_claude() {
  local src="$1" dst="$2"
  if [ ! -f "$dst" ]; then
    cp "$src" "$dst"
    echo "  ADD  $dst (new)"
    added=$((added + 1))
    return
  fi
  # Check if already appended (look for marker)
  if grep -q "ORCHESTRATOR-DELEGATION-ONLY" "$dst" 2>/dev/null; then
    echo "  SKIP CLAUDE.md (already has orchestrator section)"
    skipped=$((skipped + 1))
    return
  fi
  echo "" >> "$dst"
  cat "$src" >> "$dst"
  echo "  APPEND orchestrator rules to $dst"
  added=$((added + 1))
}

# ---- 1. Session management skills ----
echo ""
echo "--- Skills ---"
add_file "$TEMPLATE/orchestrator/.claude/skills/session-manager/SKILL.md" \
  "$DEST/.claude/skills/session-manager/SKILL.md"
add_file "$TEMPLATE/orchestrator/.claude/skills/planner/SKILL.md" \
  "$DEST/.claude/skills/planner/SKILL.md"
add_file "$TEMPLATE/orchestrator/.claude/skills/communicator/SKILL.md" \
  "$DEST/.claude/skills/communicator/SKILL.md"

# ---- 2. Session commands ----
echo ""
echo "--- Commands ---"
add_file "$TEMPLATE/orchestrator/.claude/commands/sessions.md" \
  "$DEST/.claude/commands/sessions.md"
add_file "$TEMPLATE/orchestrator/.claude/commands/spawn.md" \
  "$DEST/.claude/commands/spawn.md"
add_file "$TEMPLATE/orchestrator/.claude/commands/status.md" \
  "$DEST/.claude/commands/status.md"
add_file "$TEMPLATE/orchestrator/.claude/commands/stop.md" \
  "$DEST/.claude/commands/stop.md"

# ---- 3. MCP servers (merged) ----
echo ""
echo "--- MCP Servers ---"
merge_json "$TEMPLATE/orchestrator/.claude/settings.json" \
  "$DEST/.claude/settings.json"

# ---- 4. Orchestrator rules (appended to CLAUDE.md) ----
echo ""
echo "--- Orchestrator Rules ---"
append_claude "$TEMPLATE/orchestrator/CLAUDE.md" "$DEST/CLAUDE.md"

# ---- 5. Worker agent templates ----
echo ""
echo "--- Worker Templates ---"
mkdir -p "$DEST/.mework/templates"
cp "$TEMPLATE/orchestrator/CLAUDE.md" "$DEST/.mework/templates/orchestrator-CLAUDE.md"
echo "  COPY orchestrator reference to .mework/templates/"

# ---- 6. mzspec.config.json (if missing) ----
echo ""
echo "--- Config ---"
add_file "$TEMPLATE/mzspec.config.json" "$DEST/mzspec.config.json"

echo ""
echo "=== Done: $added added, $skipped skipped ==="
