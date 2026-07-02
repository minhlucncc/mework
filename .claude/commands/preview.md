---
name: "preview"
description: "Build a local preview server for any PR, commit, branch, or local changes"
category: Development
tags: [preview, build, test, server, docker, postgres]
---

# /preview

Build and run a local preview of the `mework-server` for a given PR, commit,
branch, or the current local state.

## Usage

```
/preview                  — use current local state
/preview <pr-number>      — checkout and preview a GitHub PR
/preview <branch>         — checkout and preview a branch
/preview <commit-hash>    — checkout and preview a specific commit
```

## Flow

### 1. Determine the target

Parse the argument:
- **No argument** → use current state (stash/dirty is OK, warn the user)
- **Numeric** (`123`) → `gh pr view 123` to confirm it exists, then `gh pr checkout 123`
- **Looks like a ref** (branch name, commit hash) → `git fetch origin` then `git checkout <arg>`. If checkout fails, report and stop.
- **Invalid/missing** → show usage

Save the previous branch name (`git rev-parse --abbrev-ref HEAD`) before
switching so you can restore it on teardown.

> **If the working tree is dirty** (uncommitted changes): `git stash push -m "preview-$(date +%s)"` first,
> note the stash ref, and `git stash pop` on teardown.

### 2. Build

```bash
make build-mework-server
```

If the build fails, stop and report the error.

### 3. Start Postgres

Check if a Postgres container is already running and usable:

```bash
docker ps --filter "name=mework-preview" --format '{{.Names}} {{.Ports}}'
```

- **Not running** → start a fresh one:

```bash
docker run -d --name mework-preview \
  -e POSTGRES_PASSWORD=preview \
  -e POSTGRES_DB=mework_preview \
  -p 5433:5432 \
  postgres:16-alpine
```

Wait for it to be ready (`pg_isready` or poll `docker logs` for
"database system is ready to accept connections").

> Port **5433** is used instead of 5432 to avoid clashing with a local
> dev/test Postgres. If 5433 is already taken, pick 5434, etc.

- **Already running** → check it's healthy (`docker exec mework-preview pg_isready -U postgres`).
  If unhealthy, restart it.

### 4. Generate secrets (if not set)

If `SERVER_KEY` or `MEWORK_SECRET_KEY` aren't in the environment, generate
32-byte random hex keys:

```bash
PREVIEW_SERVER_KEY=$(openssl rand -hex 16)
PREVIEW_SECRET_KEY=$(openssl rand -hex 16)
```

### 5. Start the preview server

Run the server **in the foreground** (background it with `&` + `run_in_background`
or a separate terminal) with the right env:

```bash
DATABASE_URL=postgres://postgres:preview@localhost:5433/mework_preview?sslmode=disable \
SERVER_KEY="${PREVIEW_SERVER_KEY:-$SERVER_KEY}" \
MEWORK_SECRET_KEY="${PREVIEW_SECRET_KEY:-$MEWORK_SECRET_KEY}" \
LISTEN_ADDR=:8080 \
  ./bin/mework-server
```

- **Background** (`&` + `run_in_background`): capture the PID and
  wait 3 seconds, then check it's still alive. If it exited, show the logs.
- Start a `Monitor` on the server's stderr/stdout so you see errors live.

### 6. Verify & report

```bash
curl -s http://localhost:8080/healthz
```

Expected: `{"status":"ok"}`

Report to the user:

> ✅ Preview server running at **http://localhost:8080**
>
> Target: `main` (or `#123: fix-login-bug`, etc.)
> Build: `abc1234` (commit hash)
> DB: `postgres://postgres:preview@localhost:5433/mework_preview`
>
> To stop: `/preview-stop`

## Teardown (`/preview-stop`)

When the user runs `/preview-stop` (or if the server dies unexpectedly):

1. Kill the `mework-server` process (SIGTERM, wait 5s, SIGKILL)
2. `docker stop mework-preview && docker rm mework-preview` (optional — ask the
   user if they want to keep the DB for debugging)
3. `git checkout <previous-branch>` (if switched)
4. `git stash pop` (if stashed)
5. Report: "Preview stopped, returned to `<branch>`."

> Create a separate `/preview-stop` command file that follows this teardown
> procedure. The preview state can be stored in a temp file like
> `/tmp/mework-preview-state.json` so `/preview-stop` knows what to clean up.
