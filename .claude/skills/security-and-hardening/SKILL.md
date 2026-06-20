---
name: security-and-hardening
description: Hardens code against vulnerabilities (mework-adapted, Go). Use when handling webhook payloads or ticket content, working with rt_token/PAT auth, sealing/unsealing provider credentials, executing AI CLIs, or integrating a new provider. Use when building any feature that accepts untrusted data, manages tokens, or interacts with provider REST APIs. Maps the OWASP Top 10 onto this repo's concrete controls (stdin-not-argv, AES-256-GCM sealing, HMAC token hashing, signature-verified webhooks, least-privilege daemon, 0600/0700 file perms).
---

# Security and Hardening

## Overview

Security-first development practices for the `mework` runtime daemon and
provider-gateway server. Treat every external input as hostile, every secret as
sacred, and every authorization check as mandatory. Security isn't a phase — it's
a constraint on every line of code that touches ticket content, tokens, provider
credentials, or the AI-CLI execution path.

This project already encodes a specific set of security controls as **invariants**
(see CLAUDE.md). They are this repo's concrete answer to the OWASP Top 10 — keep
them intact, and map any new work onto them rather than inventing parallel
mechanisms.

## The mework Security Invariants (do not break these)

| Invariant | What it defends | Where |
|---|---|---|
| **Prompts to AI CLIs go over STDIN, never argv** | Command injection — ticket content is attacker-controllable | `internal/agentrun/runner.go` |
| **Provider credentials sealed AES-256-GCM at rest**, unsealed only server-side at write-back; **daemon never holds credentials** | Secret exposure, blast radius if a dev machine is compromised | `internal/server/secret/`, write-back in `internal/server/jobs/` |
| **`rt_token` looked up via HMAC-SHA256**, never stored/compared in plaintext | Token theft from DB; timing attacks | `internal/server/token/` |
| **Webhooks signature-verified** before the payload is trusted | Spoofed events, forged enqueues | `internal/server/webhook/` |
| **PAT guards `/api/v1` management routes; `rt_token` guards `/api/v1/jobs/*`** | Broken access control, privilege confusion | `internal/server/auth/`, `internal/server/middleware/` |
| **Webhook de-dup via `UNIQUE(provider_code, external_event_id)`** | Replay / duplicate-event amplification | `jobs` schema + `jobs.Enqueue` |
| **One active job per runtime** (partial unique index); claims use `FOR UPDATE SKIP LOCKED` | Resource exhaustion, double-execution | `internal/server/jobs/` |
| **Self-retrigger guard** — never enqueue for a comment authored by the daemon's own provider user | Infinite loop / DoS amplification | webhook → enqueue path |
| **Job state machine transactional with row locks; terminal states immutable** | Tampering with job outcome | `internal/server/jobs/state.go` |
| **Config/credential files `0600`, dirs `0700`** | Local secret leakage to other users | `internal/cli/` |
| **Required server keys fail fast** (`DATABASE_URL`, `SERVER_KEY`, `MEWORK_SECRET_KEY`) | Running with missing/weak crypto config | server startup |

When you touch a subsystem, the invariant in that row is the control you must
preserve. The rest of this skill maps OWASP onto them.

## When to Use

- Building anything that accepts webhook payloads or ticket/comment content
- Working on `rt_token` / PAT authentication or authorization middleware
- Sealing/unsealing or storing provider credentials
- Executing AI CLIs (`internal/agentrun`) or building their prompts
- Adding a new provider adapter under `internal/server/provider/<name>/`
- Adding write-back to a provider REST API
- Handling any PII or credential data

## Process: Threat Model First

Controls bolted on without a threat model are guesses. Before hardening, spend
five minutes thinking like an attacker.

1. **Map the trust boundaries.** Where does untrusted data cross into the system?
   In mework: **webhook payloads**, **ticket/comment content** (which becomes the
   AI prompt), provider REST responses, CLI flags/env, and the **AI CLI's own
   output** (it later gets posted back to a provider). Every one is attack surface.
2. **Name the assets.** What's worth stealing or breaking? Provider credentials
   (sealed), `rt_token`/PAT values, the AES key (`MEWORK_SECRET_KEY`) and HMAC key
   (`SERVER_KEY`), the developer's source tree the daemon runs against, and the
   integrity of write-back (posting results as the user).
3. **Run STRIDE over each boundary** — a quick lens, not a ceremony:

| Threat | Ask | mework mitigation |
|---|---|---|
| **S**poofing | Can someone forge a webhook or a daemon? | Webhook signature verify; `rt_token` (HMAC-SHA256) on job routes; PAT on management routes |
| **T**ampering | Can data be altered in transit/at rest? | HTTPS; parameterized pgx queries; transactional job state machine with row locks; sealed credentials |
| **R**epudiation | Can an action be denied later? | Job rows + state transitions are an audit trail; de-dup key ties an event to its job |
| **I**nformation disclosure | Can secrets leak? | AES-256-GCM sealing; HMAC token hashing; `0600`/`0700` perms; never log secrets; daemon never holds credentials |
| **D**enial of service | Can it be overwhelmed? | One active job per runtime; `FOR UPDATE SKIP LOCKED`; de-dup; self-retrigger guard; 30m run cap; HTTP timeouts |
| **E**levation of privilege | Can a caller gain rights it shouldn't? | PAT vs `rt_token` route separation; least-privilege daemon (no credentials) |

4. **Write abuse cases next to use cases.** For each feature, ask "how would I
   misuse this?" — then make that your first table-driven test (`net/http/httptest`
   for handlers; `internal/integration` for the full pipeline).

If you can't name the trust boundaries for a feature, you're not ready to secure
it. This is OWASP **A04: Insecure Design** — most breaches begin in design.

## The Three-Tier Boundary System

### Always Do (No Exceptions)

- **Validate all external input** at the boundary — webhook handlers, `ParseTrigger`, CLI flag parsing.
- **Pass prompts to AI CLIs over STDIN, never argv.** Ticket content is hostile.
- **Parameterize all pgx queries** (`$1`, `$2`) — never concatenate input into SQL.
- **Verify webhook signatures** before acting on a payload.
- **Look up `rt_token` via HMAC-SHA256**; compare with a constant-time compare; never store plaintext.
- **Seal provider credentials with AES-256-GCM** at rest; unseal only server-side, only at write-back time.
- **Use HTTPS** for all provider and server communication.
- **Fail fast** when `DATABASE_URL`, `SERVER_KEY`, or `MEWORK_SECRET_KEY` is missing.
- **Set file perms `0600`** for credential/config files, **`0700`** for dirs.
- **Run `govulncheck ./...`** (or equivalent) before a release; keep `go.sum` committed.

### Ask First (Requires Human Approval)

- Adding or changing authentication/authorization logic (PAT or `rt_token` paths)
- Changing the credential sealing scheme or key handling
- Adding a new provider integration or changing webhook signature verification
- Loosening route guards or CORS
- Changing rate limiting, the de-dup key, the per-runtime job limit, or the run timeout
- Granting the daemon any access to provider credentials (it should never have them)
- Changing what gets logged near tokens, keys, or credentials

### Never Do

- **Never put ticket content (or any untrusted data) on the AI CLI's argv.**
- **Never commit secrets** — AES/HMAC keys, PATs, `rt_token` values, provider credentials.
- **Never log sensitive data** — tokens, keys, unsealed credentials, or the full prompt.
- **Never store or compare `rt_token`/PAT in plaintext.**
- **Never let the daemon hold provider credentials.**
- **Never trust a webhook payload before its signature is verified.**
- **Never expose stack traces or internal errors** to provider write-back or HTTP responses.
- **Never substitute a default for a missing required crypto key** — fail fast instead.

## OWASP Top 10, Mapped onto mework

These are prevention patterns expressed in this repo's terms.

### A03 Injection — and the #1 mework rule

The headline injection surface here is **command injection via the AI CLI**, not
SQL. Ticket and comment content is attacker-controllable and becomes the model
prompt. **It must reach the CLI over STDIN, never as a command-line argument.**

```go
// BAD: ticket content on argv — shell/arg injection, content interpreted as flags
cmd := exec.CommandContext(ctx, cliBin, "--prompt", ticketContent)

// GOOD: prompt over stdin; argv carries only fixed, trusted flags
cmd := exec.CommandContext(ctx, cliBin, fixedFlags...)
cmd.Stdin = strings.NewReader(prompt) // attacker-controllable bytes never touch argv
cmd.Dir = isolatedWorkdir             // isolated workdir, bounded by the 30m timeout
```

For Postgres, always parameterize:

```go
// BAD: string-concatenated SQL
rows, _ := db.Query(ctx, "SELECT * FROM jobs WHERE provider_code = '"+code+"'")

// GOOD: parameterized
rows, _ := db.Query(ctx, "SELECT * FROM jobs WHERE provider_code = $1", code)
```

### A07 Identification & Authentication Failures — tokens

`rt_token` and PAT are bearer credentials. Store only a keyed hash; compare in
constant time.

```go
// rt_token lookup: hash with the server HMAC key, look up by hash, constant-time verify.
func lookupHash(token string, serverKey []byte) []byte {
    mac := hmac.New(sha256.New, serverKey)
    mac.Write([]byte(token))
    return mac.Sum(nil) // store/compare THIS, never the raw token
}

// Verifying: HMAC the presented token, compare to the stored hash in constant time.
if !hmac.Equal(presentedHash, storedHash) {
    return errUnauthorized
}
```

Never log the raw token. Generate tokens from a CSPRNG (`crypto/rand`).

### A01 Broken Access Control — route guards

Authentication is not authorization. Enforce the right token type per route and
verify ownership.

- PAT middleware guards `/api/v1` management routes (runtimes, connections, profiles).
- `rt_token` middleware guards `/api/v1/jobs/*` (claim/ack/heartbeat).
- `/webhooks/{provider}` is signature-verified (not token-auth'd); `/healthz` is open.

A daemon presenting a valid `rt_token` must only be able to act on its own jobs —
the one-active-job-per-runtime index and claim semantics enforce this. Don't let a
job route accept a PAT or vice versa.

### A02 Cryptographic Failures — credential sealing

Provider credentials are sealed with AES-256-GCM and unsealed only server-side at
write-back. The daemon never sees them.

```go
// Seal (server-side, at storage time)
func seal(key, plaintext []byte) ([]byte, error) {
    block, err := aes.NewCipher(key) // 32-byte key from MEWORK_SECRET_KEY
    if err != nil { return nil, err }
    gcm, err := cipher.NewGCM(block)
    if err != nil { return nil, err }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := rand.Read(nonce); err != nil { return nil, err } // crypto/rand
    return gcm.Seal(nonce, nonce, plaintext, nil), nil // nonce prepended; AEAD authenticates
}
```

GCM is authenticated (AEAD), so tampering is detected on unseal. Never reuse a
nonce with the same key. Keys come from env (`MEWORK_SECRET_KEY`,
`SERVER_KEY`) and the server fails fast if they're missing.

### A04 Insecure Design — the invariants are the design

The de-dup unique key, the per-runtime job limit, the self-retrigger guard, the
immutable terminal states, and the stdin-not-argv rule are design-level controls.
A "clever" shortcut that bypasses one of them is an insecure design even if each
line of code is fine. Preserve them; if a feature seems to require breaking one,
that's an Ask-First.

### A05 Security Misconfiguration

- Required crypto config (`DATABASE_URL`, `SERVER_KEY`, `MEWORK_SECRET_KEY`) **fails fast** — don't add silent defaults.
- File perms: credential/config files `0600`, dirs `0700`. Verify on create.
- Don't leak internals: write-back and HTTP error responses carry generic messages, not stack traces or DB errors.
- Keep optional surfaces tight (`WEBHOOK_SECRET`, `MELLO_BASE_URL`, `LISTEN_ADDR`).

### A08 Software & Data Integrity — webhook signatures + supply chain

```go
// Verify the provider's signature BEFORE parsing the trigger or enqueuing.
if !verifySignature(rawBody, sigHeader, webhookSecret) {
    http.Error(w, "invalid signature", http.StatusUnauthorized)
    return
}
// only now: ParseTrigger(rawBody), jobs.Enqueue(...)
```

Use a constant-time compare for the signature. For supply chain: commit `go.sum`,
install reproducibly in CI, run `govulncheck ./...`, and review every new module
(maintenance, license, footprint) — see the `code-review-and-quality` sibling
skill's dependency discipline section.

### A09 Security Logging & Monitoring Failures

Log security-relevant events (claim, state transition, signature-verify failure,
auth failure) with job/request context — but **never** log token values, keys,
unsealed credentials, or the full prompt. The job rows and state transitions are
your audit trail.

### A10 SSRF — provider base URLs and write-back targets

The server fetches provider REST endpoints (e.g. `MELLO_BASE_URL`, write-back
URLs). If any provider/connection config lets a user influence the host, an
attacker can aim it at internal services (cloud metadata `169.254.169.254`,
`localhost`, private ranges).

```go
// Allowlist scheme + host, reject private/reserved resolved IPs, forbid redirects.
func assertSafeURL(raw string, allowedHosts map[string]bool) (*url.URL, error) {
    u, err := url.Parse(raw)
    if err != nil { return nil, err }
    if u.Scheme != "https" { return nil, errors.New("https only") }
    if !allowedHosts[u.Hostname()] { return nil, errors.New("host not allowed") }
    ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), u.Hostname())
    if err != nil { return nil, err }
    for _, ip := range ips {
        if ip.IP.IsLoopback() || ip.IP.IsPrivate() || ip.IP.IsLinkLocalUnicast() ||
            ip.IP.IsUnspecified() {
            return nil, errors.New("private/reserved IP")
        }
    }
    return u, nil
}
// Use an http.Client whose CheckRedirect returns an error to forbid redirects.
```

Note the TOCTOU gap: Go re-resolves DNS on dial, so a short-TTL record can rebind
between check and connect. For high-risk targets, pin the validated IP via a
custom `DialContext`, or keep provider hosts to a fixed allowlist.

## Treating AI CLI Output as Untrusted (OWASP LLM Top 10)

mework runs AI coding CLIs and posts their output back to providers. That output —
and the ticket content that prompts it — is a fresh attack surface.

- **Prompt injection (LLM01).** Ticket/comment content in the prompt can carry
  instructions. The prompt is not a security boundary; enforce permissions in code
  (route guards, the sandboxed/isolated workdir, the run timeout), not in prompt
  text. This is *why* the content goes over stdin and the workdir is isolated.
- **Improper output handling (LLM05).** The CLI's result is posted back via the
  provider REST API. Treat it as untrusted: it's data to write into a comment, not
  a command to run. Never feed model output into a shell, SQL, or eval. Bound its
  size before write-back.
- **Sensitive info disclosure (LLM02/LLM07).** Don't put `rt_token`s, the AES/HMAC
  keys, unsealed credentials, or another tenant's data into the prompt — anything
  in context can be echoed back into a public comment. The daemon's
  credential-free design helps here: it has nothing to leak.
- **Excessive agency (LLM06).** The CLI runs in an isolated workdir under a 30m
  timeout; keep that scoping. Destructive/irreversible provider actions should be
  deliberate, not implied by model output.
- **Unbounded consumption (LLM10).** The per-runtime job limit, de-dup key,
  self-retrigger guard, and run timeout cap cost and prevent loops — don't weaken
  them.

## Input Validation Patterns

Validate at the boundary before anything trusts the value.

```go
// Webhook trigger: ParseTrigger enforces the grammar; reject anything that doesn't match.
//   @mework [profile] [workflow] [instructions], workflow ∈ plan|cook|test|review|ship|journal
trig, ok := webhook.ParseTrigger(comment)
if !ok {
    return // not a trigger; do not enqueue
}
// Self-retrigger guard: never enqueue for the daemon's own provider user.
if trig.AuthorID == daemonProviderUserID {
    return
}
```

Bound sizes (comment length, prompt size, result size). Reject before use, not
after.

## Secrets Management

```
Environment (server, fail-fast if missing):
  ├── DATABASE_URL          Postgres DSN
  ├── SERVER_KEY            HMAC key for rt_token lookup hashing
  └── MEWORK_SECRET_KEY     AES-256-GCM key for sealing provider credentials
Optional: WEBHOOK_SECRET, MELLO_BASE_URL, LISTEN_ADDR

Local (CLI/daemon):
  ~/.mework/config.json     0600 file, 0700 dir  (never holds provider credentials)

.gitignore must exclude:
  *.pem  *.key  .env  .env.local  any config holding real secrets
```

**Check before committing:**
```bash
git diff --cached | grep -iE "rt_token|password|secret|api[_-]?key|BEGIN .*PRIVATE KEY|MEWORK_SECRET_KEY|SERVER_KEY"
```

**If a secret is ever committed, rotate it.** Deleting the line or rewriting
history is not enough — assume it's compromised the moment it reaches a remote.
Revoke and reissue (rotate the AES/HMAC key, reissue affected `rt_token`s and
provider credentials), then purge from history.

## Triaging govulncheck / Dependency Findings

```
govulncheck reports a vulnerability
├── Is the vulnerable symbol actually reachable in our code path?
│   ├── YES + high severity --> Fix immediately (update or replace the module)
│   └── NO (unreachable / build-only) --> govulncheck already tells you; fix soon, not a blocker
├── Is a fixed version available?
│   ├── YES --> bump in go.mod, run go mod tidy, re-run make build && make test
│   └── NO  --> workaround, replace the dependency, or allowlist with a review date
└── Track lower-severity items in the backlog and clear them during regular updates.
```

`govulncheck` is reachability-aware, which is its advantage over a plain CVE list.
It still won't catch a malicious/typosquatted module — review new dependencies
before adding them (see `code-review-and-quality`). Commit `go.sum`; install
reproducibly in CI.

## Security Review Checklist

```markdown
### AI CLI execution
- [ ] Prompt passed over STDIN, never argv
- [ ] CLI runs in an isolated workdir under the run timeout (30m)
- [ ] CLI output bounded and treated as untrusted before write-back

### Authentication & tokens
- [ ] rt_token stored/compared as HMAC-SHA256, never plaintext; constant-time compare
- [ ] Tokens generated from crypto/rand
- [ ] PAT guards /api/v1; rt_token guards /api/v1/jobs/*; webhooks signature-verified

### Authorization
- [ ] Each route enforces the correct token type
- [ ] A daemon can only act on its own jobs (one-active-job index, claim semantics)

### Input
- [ ] Webhook signature verified BEFORE ParseTrigger/Enqueue
- [ ] Trigger grammar enforced; self-retrigger guard in place
- [ ] pgx queries parameterized
- [ ] Server-side URL fetches allowlisted (no SSRF to internal services)

### Secrets & data
- [ ] No secrets/tokens/keys/credentials in code, logs, or git history
- [ ] Provider credentials sealed AES-256-GCM; daemon never holds them
- [ ] Required keys (DATABASE_URL, SERVER_KEY, MEWORK_SECRET_KEY) fail fast
- [ ] Config/credential files 0600, dirs 0700

### Integrity & supply chain
- [ ] go.sum committed; CI installs reproducibly; govulncheck clean of reachable high/critical
- [ ] New dependencies reviewed (maintenance, license, footprint)

### Errors & logging
- [ ] No stack traces / internal errors in HTTP responses or write-back
- [ ] Security events logged with context; no secrets in the logs
```

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "It's just ticket text, putting it on argv is fine" | Ticket text is attacker-controllable. On argv it's command injection. STDIN, always. |
| "This is an internal tool, security doesn't matter" | The daemon runs against a developer's real source tree and the server holds every provider's credentials. The blast radius is large. |
| "We'll add security later" | Retrofitting sealing, token hashing, and signature verification is 10x harder than building them in. The invariants exist so you don't have to. |
| "Just log the token while I debug" | A logged token is a leaked token. Log the job ID, never the secret. |
| "The daemon can just cache the provider credential" | No. The daemon never holds provider credentials — that's the whole point of server-side sealing/unsealing. |
| "Threat modeling is overkill here" | Five minutes of "how would I attack this webhook?" prevents the design flaws no control can patch later. |
| "It's only the model's output, it's just text" | That text gets posted back to a provider and could be a shell command or markup. Treat it as untrusted. |

## Red Flags

- Ticket/comment content reaching the AI CLI via argv, a shell string, or an env var
- Secrets, `rt_token`s, the AES/HMAC key, or provider credentials in source, logs, or history
- `rt_token`/PAT stored or compared in plaintext, or a non-constant-time compare
- The daemon obtaining or caching provider credentials
- A webhook payload parsed/enqueued before signature verification
- A job route accepting a PAT (or a management route accepting an `rt_token`)
- String-concatenated pgx queries
- A new provider that requires a schema migration (breaks the provider-agnostic invariant) or bypasses the de-dup key
- Server-side fetch of a user-influenced URL without an allowlist (SSRF)
- Required crypto keys defaulted instead of failing fast
- Config/credential files created without `0600` / dirs without `0700`

## mework notes

- **Invariants ARE the controls.** The table at the top of this skill is this
  repo's OWASP answer: stdin-not-argv (A03), AES-256-GCM sealing (A02),
  HMAC-SHA256 `rt_token` hashing (A07), signature-verified webhooks (A08), PAT vs
  `rt_token` route guards (A01), de-dup + per-runtime limit + self-retrigger guard
  + run timeout (A10/DoS, LLM10), immutable terminal job states (tampering), and
  `0600`/`0700` perms (A05). Preserve them; breaking one is an Ask-First.
- **Lifecycle:** security-affecting changes go through OpenSpec
  (`/opsx:propose` → `/opsx:apply` → `/opsx:sync` → `/opsx:archive`). Run the
  built-in `/security-review` and `/code-review` commands; the autonomous
  `/opsx:ship` pipeline gates on `make vet` + `make test`.
- **Verify with:** `make vet`, `make build`, `make test` (`go test -p 1 ./...`,
  serialized; DB tests skip without `TEST_DATABASE_URL`; start Postgres with
  `make test-db`), plus `govulncheck ./...`. Drive handler-level abuse cases with
  `net/http/httptest` and full-pipeline ones with `internal/integration`
  (`TestFullPipelineE2E`).
- See the `code-review-and-quality` and `debugging-and-error-recovery` sibling
  skills for the review-axis and root-cause depth this skill references.

## Verification

After implementing security-relevant code:

- [ ] Prompts reach the AI CLI over STDIN, never argv; CLI runs in an isolated workdir under the timeout
- [ ] `govulncheck ./...` shows no reachable critical/high vulnerabilities; `go.sum` committed
- [ ] No secrets/tokens/keys/credentials in source or git history
- [ ] `rt_token` lookups via HMAC-SHA256 with constant-time compare; tokens from `crypto/rand`
- [ ] Provider credentials sealed AES-256-GCM; daemon holds none
- [ ] Webhook signatures verified before enqueue; de-dup + self-retrigger guard intact
- [ ] Correct token type enforced per route; required keys fail fast
- [ ] Config/credential files `0600`, dirs `0700`
- [ ] Server-side URL fetches validated against an allowlist (no SSRF)
- [ ] Error responses and write-back carry no stack traces or internal details
- [ ] `make vet`, `make build`, `make test` pass (DB tests actually ran)
