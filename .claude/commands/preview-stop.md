---
name: "preview-stop"
description: "Stop the preview server and clean up resources"
category: Development
tags: [preview, stop, cleanup, docker]
---

# /preview-stop

Stop the preview server started by `/preview` and clean up.

## Flow

### 1. Load preview state

Check `/tmp/mework-preview-state.json` for saved state:

```bash
cat /tmp/mework-preview-state.json 2>/dev/null
```

Fields: `pid`, `target`, `previous_branch`, `stash_ref`, `docker_container`,
`db_port`.

If the file is missing, try to find the process anyway (grep for
`bin/mework-server` in `ps aux`).

### 2. Stop the server

If a PID is known or found:

```bash
kill -TERM <pid> 2>/dev/null
sleep 3
kill -KILL <pid> 2>/dev/null
```

Confirm it's gone (`! kill -0 <pid> 2>/dev/null`).

### 3. Clean up Docker

Ask the user:

> Keep the Postgres container for debugging? (yes/No)

- **No** (default): `docker stop mework-preview && docker rm mework-preview`
- **Yes**: leave it running

### 4. Restore git state

```bash
git checkout <previous-branch>     # if a switch happened
git stash pop                      # if stashed
```

Note: if there are now uncommitted changes from the preview build (e.g. `bin/`
directory, modified go.sum), `git checkout` may warn. Use `git checkout --force`
if the user confirms, or just note the conflicts.

### 5. Report

> 🧹 Preview stopped. Returned to `<branch>`.
> Postgres container: kept / removed
