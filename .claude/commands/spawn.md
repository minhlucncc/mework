---
name: "spawn"
description: "Spawn a new child sandbox worker"
category: Orchestrator
tags: [spawn, create, sandbox, worker]
---

# /spawn

Create a child sandbox worker. This is a MANDATORY tool call — do NOT create
sessions manually or simulate spawning.

Usage:

`/spawn <agent_id> [task description]`

1. Extract the agent_id (first word) and task prompt (rest of message).
2. If no task given, use "cat" as the prompt (reads from stdin indefinitely).
3. **Call this tool now:** `mcp__mework-mcp__spawn_sandbox`
   with arguments `{"agent_id": "<name>", "prompt": "<task or 'cat'>"}`
4. **Then call:** `mcp__mework-mcp__wait_for_sandbox` with the returned id.
5. Report the sandbox output to the user.
