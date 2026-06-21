package runner

import (
	"context"
	"errors"
	"log"
	"time"

	"mework/client/subscribe"
	"mework/sandbox/agent"
	"mework/sandbox/engine/local"
	"mework/shared/config"
)

// Run is the daemon's main loop: poll the mework-server claim API,
// run the AI agent, heartbeat during run, and ack completion.
func Run(ctx context.Context, profile string, cfg *config.Config) error {
	if cfg.ServerURL == "" {
		return errors.New("server_url is not set in config; please configure it first")
	}
	if cfg.RuntimeToken == "" {
		return errors.New("rt_token is not set in config; please register this runtime first")
	}

	backend, ok := agent.Detect(cfg.Daemon.Backends)
	if !ok {
		log.Printf("warning: no AI CLI detected (looked for %v); triggers will be skipped until backend is installed", agent.DefaultBackends)
	} else {
		log.Printf("using AI backend %s (%s)", backend.Name, backend.Path)
	}

	client := subscribe.NewClient(cfg.ServerURL, 10*time.Second)

	pollInterval := 5 * time.Second
	if cfg.Daemon.PollIntervalSeconds > 0 {
		pollInterval = time.Duration(cfg.Daemon.PollIntervalSeconds) * time.Second
	}

	log.Printf("daemon polling server %s every %s", cfg.ServerURL, pollInterval)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("daemon stopping")
			return nil
		case <-ticker.C:
			if !ok {
				backend, ok = agent.Detect(cfg.Daemon.Backends)
				if !ok {
					continue
				}
				log.Printf("detected AI backend %s (%s)", backend.Name, backend.Path)
			}

			job, err := client.Claim(cfg.RuntimeToken)
			if err != nil {
				log.Printf("error claiming job: %v. Retrying in next cycle...", err)
				continue
			}

			if job == nil {
				continue
			}

			log.Printf("claimed job %s for task %s", job.ID, job.ExternalTaskID)

			// Transition to running
			if err := client.Ack(cfg.RuntimeToken, job.ID, "running", "", ""); err != nil {
				log.Printf("error acking running status for job %s: %v", job.ID, err)
				continue
			}

			// Start heartbeat (extends lease in background every 30s)
			stopHeartbeat := startHeartbeat(ctx, client, cfg.RuntimeToken, job.ID, 30*time.Second)

			// Prepare prompt
			prompt := buildPrompt(job)
			workDir := local.WorkDir(config.ProfileDir(profile), job.ID)

			// Execute AI agent
			res := local.Run(ctx, backend, prompt, workDir, 30*time.Minute)

			// Stop heartbeat
			stopHeartbeat()

			// Terminal transition
			status := "done"
			var lastError string
			if res.Err != nil {
				status = "failed"
				lastError = res.Err.Error()
			}
			summary := formatResult(backend.Name, res)

			if err := client.Ack(cfg.RuntimeToken, job.ID, status, summary, lastError); err != nil {
				log.Printf("error acking terminal status %s for job %s: %v", status, job.ID, err)
			} else {
				log.Printf("job %s completed (status=%s)", job.ID, status)
			}
		}
	}
}

func startHeartbeat(ctx context.Context, client *subscribe.Client, rtToken, jobID string, interval time.Duration) func() {
	hbCtx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				if err := client.Heartbeat(rtToken, jobID); err != nil {
					log.Printf("Heartbeat failed for job %s: %v", jobID, err)
				}
			}
		}
	}()

	return cancel
}
