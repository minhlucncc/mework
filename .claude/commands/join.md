---
name: "join"
description: "Attach directly to a running sandbox's stdin/stdout"
category: Orchestrator
tags: [join, attach, sandbox, direct]
---

# /join

Attach your chat directly to a running sandbox's stdin/stdout, bypassing the
orchestrator. Messages go directly to the sandbox.

Usage:

`/join <sandbox_id_or_name>`

1. First call `mcp__mework-mcp__list_child_sandboxes` with `parent_id: "mework-dev"`
   to find your sandboxes.
2. If given a name (e.g. "coding"), find the sandbox with that agent_id.
3. Then output: "Joined sandbox <id>." so the chat client shows the indicator.
4. All subsequent messages go directly to that sandbox.
