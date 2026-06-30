// Binary mework-mezon-worker is a standalone worker that connects to Mezon via
// the bot library, enqueues received messages as jobs via the mework-server API,
// and polls for completed jobs to send replies back to Mezon channels.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	mezonbot "mework/libs/server/provider/mezon/bot"
	sharedMezon "mework/libs/shared/providers/mezon"
)

func main() {
	cfg, err := Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Create the bot with nil SDK client (no-op until a real SDK is wired in).
	bot := mezonbot.New(
		sharedMezon.Config{
			AppID:   cfg.MezonAppID,
			APIKey:  cfg.MezonAPIKey,
			BaseURL: cfg.MezonBaseURL,
		},
		nil,
		func(msg mezonbot.Message) {},
	)

	w := New(cfg, bot)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received signal %v, shutting down", sig)
		cancel()
	}()

	if err := w.Run(ctx); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
