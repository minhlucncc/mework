# Mezon Worker

## Purpose

Define the standalone `mework-mezon-worker` binary that connects to Mezon via WebSocket, enqueues received channel messages as jobs on the mework server, and polls the server for completed jobs to send replies back to Mezon channels. The worker replaces the server-embedded Mezon bot and the daemon-hosted Mezon proxy, running as an independent process that communicates with the mework server over HTTP API.

## Requirements

### Requirement: Worker enqueues Mezon messages as jobs

The worker SHALL connect to Mezon via WebSocket, receive channel messages (the bot library filters self-messages before dispatch), and enqueue each message as a job via the server API. The enqueue payload SHALL include the provider code `"mezon"`, the channel ID, sender ID, message text, and message ID for dedup.

#### Scenario: Message received and enqueued

- **WHEN** the worker receives a Mezon channel message from another user
- **THEN** it calls `POST /api/v1/jobs/enqueue` with `{provider_code: "mezon", channel_id, sender_id, text, message_id}`
- **THEN** the server creates a job with status `queued` and `provider_code = "mezon"`

#### Scenario: Enqueue endpoint returns server error

- **WHEN** the `POST /api/v1/jobs/enqueue` call returns a 5xx error
- **THEN** the worker logs the error and continues without crashing
- **THEN** the message is dropped (not retried or queued locally)

#### Scenario: Initial Mezon connection fails

- **WHEN** the worker cannot connect to Mezon WebSocket on startup due to invalid credentials or unreachable host
- **THEN** the worker logs the error and exits with a non-zero exit code

#### Scenario: Permanent disconnection after retries

- **WHEN** the Mezon WebSocket reconnection attempts exceed the retry limit
- **THEN** the inbound loop stops and logs a fatal error
- **THEN** the outbound loop continues operating independently

### Requirement: Worker polls for completed jobs at a configurable interval

The worker SHALL poll the server for completed jobs where `provider = "mezon"` at a configurable interval (default 5s).

#### Scenario: Poll and reply

- **WHEN** the worker polls `GET /api/v1/jobs?provider=mezon&status=done&since=<cursor>` and finds a completed job
- **THEN** it extracts the result and channel ID from the job
- **THEN** it posts the result as a message to the Mezon channel via the bot's `SendMessage()`
- **THEN** it advances the cursor past that job ID

### Requirement: Worker deduplicates replies via persisted cursor

The worker SHALL use a cursor to avoid processing the same job twice and SHALL persist the cursor to a local file for crash recovery.

#### Scenario: Cursor persisted across restarts

- **WHEN** the worker restarts after a crash
- **THEN** it reads the cursor from the local state file and resumes polling from that point
- **THEN** already-replied jobs before the cursor are not re-processed

#### Scenario: SendMessage failure does not advance cursor

- **WHEN** `SendMessage` returns an error during the outbound reply
- **THEN** the cursor is NOT advanced past that job ID
- **THEN** the job is re-processed on the next poll interval

### Requirement: Worker is configured via environment

The worker SHALL read its configuration from environment variables: `MEZON_APP_ID`, `MEZON_API_KEY`, `MEZON_BASE_URL` (optional), `MEWORK_SERVER_URL` (default `http://localhost:8080`), `MEWORK_TOKEN` (runtime token for API auth), and `REDIS_URL` (optional, default: embedded in-memory Redis). When `REDIS_URL` is unset, the worker SHALL start an embedded in-memory Redis server (`miniredis`) for the turbo engine's state store (message dedup, channel cursors, activity tracking). State is lost on restart when using the embedded fallback.

#### Scenario: Start with minimal config

- **WHEN** the worker starts with `MEZON_APP_ID`, `MEZON_API_KEY`, `MEWORK_TOKEN`, and `MEWORK_SERVER_URL` set
- **THEN** it connects to Mezon and begins the inbound and outbound loops

#### Scenario: Missing credentials fails fast

- **WHEN** the worker starts without `MEZON_APP_ID`, `MEZON_API_KEY`, or `MEWORK_TOKEN`
- **THEN** it prints an error listing the missing variables and exits with a non-zero exit code

#### Scenario: Worker starts without Redis

- **WHEN** the worker starts without `REDIS_URL` set
- **THEN** it starts an embedded miniredis server
- **THEN** it logs a warning: `WARNING: using embedded in-memory state, lost on restart`
- **THEN** the turbo engine operates normally with in-memory state

#### Scenario: Worker starts with Redis

- **WHEN** the worker starts with `REDIS_URL` set to a valid Redis URL
- **THEN** it connects to the external Redis server
- **THEN** state is persistent across restarts

#### Scenario: Worker restarts without Redis

- **WHEN** the worker running with miniredis is restarted
- **THEN** all state (dedup cursors, channel tracking, activity) is lost
- **THEN** the worker re-learns channels from inbound messages
- **THEN** duplicate message delivery may occur for messages seen before the restart

### Requirement: Isolated inbound and outbound loops

The worker SHALL run the inbound (receive -> enqueue) and outbound (poll -> reply) loops as independent concurrent tasks. A failure in one loop SHALL NOT affect the other.

#### Scenario: Inbound loop fails independently

- **WHEN** the Mezon WebSocket connection drops and the inbound loop reconnects
- **THEN** the outbound loop continues polling and replying without interruption

#### Scenario: Outbound poll error is logged

- **WHEN** the outbound loop's poll request to the server fails
- **THEN** the error is logged and the loop retries on the next interval
- **THEN** the inbound loop continues processing messages

### Requirement: Worker logs and continues on non-fatal errors

The worker SHALL log errors from either loop and continue operating. Only errors that make continued operation impossible (e.g., permanent credential rejection) SHALL cause the worker to exit.

#### Scenario: Server API transient error is logged

- **WHEN** a call to the server API returns a transient error (timeout, 503)
- **THEN** the worker logs the error and the affected loop retries
- **THEN** the worker does not exit

### Requirement: Worker sends replies via bot client

The worker SHALL send replies to Mezon channels by calling `bot.SendMessage()` directly, not through the mezon-provider adapter's WriteBack method. The outbound loop SHALL extract the channel ID and result text from the completed job and pass them to the bot client's `SendMessage(ctx, channelID, content)`.

#### Scenario: Outbound reply via bot

- **WHEN** the outbound loop finds a completed job for a Mezon channel
- **THEN** it extracts `channel_id` and the result text from the job record
- **THEN** it calls `bot.SendMessage(ctx, channelID, resultText)`
- **THEN** on success, it advances the cursor past that job
