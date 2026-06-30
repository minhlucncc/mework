package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	mezonbot "mework/libs/server/provider/mezon/bot"
)

// Worker runs inbound (receive -> enqueue) and outbound (poll -> reply) loops.
type Worker struct {
	cfg        *Config
	bot        *mezonbot.Bot
	httpClient *http.Client
	cursor     *Cursor
	wg         sync.WaitGroup
	cancel     context.CancelFunc
	botUserID  string // authenticated bot user ID for self-message filtering
}

// Cursor tracks the last processed job ID on disk for crash recovery.
type Cursor struct {
	path string
	mu   sync.Mutex
}

// doneJob represents a completed job returned by GET /api/v1/jobs.
type doneJob struct {
	ID            string `json:"id"`
	ChannelID     string `json:"channel_id"`
	ResultSummary string `json:"result_summary"`
	Status        string `json:"status"`
}

// New creates a Worker and registers its inbound message handler on the bot.
func New(cfg *Config, bot *mezonbot.Bot) *Worker {
	w := &Worker{
		cfg: cfg,
		bot: bot,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
		cursor: &Cursor{path: cfg.CursorPath},
	}

	// Register the inbound message handler: every received Mezon message is
	// enqueued as a job via the server API, except self-authored messages.
	bot.OnMessage(func(ctx context.Context, channelID, senderID, text string) {
		// Self-retrigger guard: never enqueue messages sent by the bot itself.
		if senderID == w.botUserID {
			return
		}
		// The bot's Message struct does not carry a message ID, so we derive
		// one for dedup.
		messageID := fmt.Sprintf("%s:%s:%d", channelID, senderID, time.Now().UnixNano())
		w.enqueueJob(ctx, channelID, senderID, text, messageID)
	})

	return w
}

// Run starts the inbound and outbound loops as goroutines and blocks until ctx
// is cancelled, then performs a graceful shutdown.
func (w *Worker) Run(ctx context.Context) error {
	ctx, w.cancel = context.WithCancel(ctx)
	defer w.cancel()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.InboundLoop(ctx)
	}()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.OutboundLoop(ctx)
	}()

	<-ctx.Done()
	w.wg.Wait()
	return nil
}

// InboundLoop authenticates, connects, and starts the bot's dispatch loop.
// On failure (auth or connect), it logs and returns without crashing the
// worker. The bot.Start call blocks until ctx is cancelled.
func (w *Worker) InboundLoop(ctx context.Context) {
	log.Println("inbound: starting")

	if err := w.bot.Authenticate(); err != nil {
		log.Printf("inbound: authenticate failed: %v", err)
		return
	}

	// Read the bot's authenticated user ID for self-message filtering.
	// The bot exposes this via its exported field after Authenticate.
	if userID := w.bot.UserID(); userID != "" {
		w.botUserID = userID
	}

	if err := w.bot.Connect(); err != nil {
		log.Printf("inbound: connect failed: %v", err)
		return
	}

	// bot.Start blocks until ctx is cancelled.
	if err := w.bot.Start(ctx); err != nil {
		log.Printf("inbound: bot start exited: %v", err)
	}
}

// OutboundLoop polls the server for completed Mezon jobs at the configured
// interval and sends replies via the bot. Errors are logged and the loop
// continues.
func (w *Worker) OutboundLoop(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.pollAndProcess(ctx)
		}
	}
}

// pollAndProcess fetches completed Mezon jobs from the server and processes
// them (send reply, advance cursor).
func (w *Worker) pollAndProcess(ctx context.Context) {
	jobs, err := w.pollDoneJobs(ctx)
	if err != nil {
		log.Printf("outbound: poll error: %v", err)
		return
	}



	for _, job := range jobs {
		// Send the result to the Mezon channel via the bot.
		if err := w.bot.SendMessage(ctx, job.ChannelID, job.ResultSummary); err != nil {
			log.Printf("outbound: send message error: %v", err)
			// Do not advance cursor on failure.
			continue
		}

		// Advance cursor on successful reply.
		if err := w.cursor.Save(job.ID); err != nil {
			log.Printf("outbound: save cursor error: %v", err)
		}
	}
}

// pollDoneJobs calls GET /api/v1/jobs with provider=mezon, status=done, and
// the current cursor as the since parameter.
func (w *Worker) pollDoneJobs(ctx context.Context) ([]doneJob, error) {
	cursor, _ := w.cursor.Load()

	url := fmt.Sprintf("%s/api/v1/jobs?provider=mezon&status=done&since=%s",
		w.cfg.MeworkServerURL, cursor)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+w.cfg.MeworkToken)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("poll returned HTTP %d", resp.StatusCode)
	}

	var jobs []doneJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, err
	}

	return jobs, nil
}

// enqueueJob sends a POST /api/v1/jobs/enqueue request to the server with the
// message details. Errors are logged; the method never crashes the worker.
func (w *Worker) enqueueJob(ctx context.Context, channelID, senderID, text, messageID string) {
	payload := map[string]string{
		"provider_code": "mezon",
		"channel_id":    channelID,
		"sender_id":     senderID,
		"text":          text,
		"message_id":    messageID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("enqueue: marshal error: %v", err)
		return
	}

	url := w.cfg.MeworkServerURL + "/api/v1/jobs/enqueue"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("enqueue: request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.cfg.MeworkToken)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		log.Printf("enqueue: http error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("enqueue: server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		return
	}

	_, _ = io.Copy(io.Discard, resp.Body)
}

// Load reads the cursor from the file on disk. Returns empty string if the
// file does not exist.
func (c *Cursor) Load() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Save writes the job ID to the cursor file on disk.
func (c *Cursor) Save(jobID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return os.WriteFile(c.path, []byte(jobID), 0600)
}
