package daemon

import (
	"context"
	"log"

	"mework/internal/cli"
)

// Run is the daemon's main loop entrypoint. Phase 06 provides a lifecycle-only
// implementation that blocks until the context is cancelled; phase 07 replaces
// the body with the poll-based trigger loop.
func Run(ctx context.Context, profile string, cfg *cli.Config) error {
	log.Printf("mello daemon started (profile=%q)", profile)
	<-ctx.Done()
	log.Printf("mello daemon stopping (profile=%q)", profile)
	return nil
}
