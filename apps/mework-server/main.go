package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mework/libs/server/hub"
	mezoncfg "mework/libs/shared/providers/mezon"
	"mework/libs/server/platform/store"
	mezonbot "mework/libs/server/provider/mezon/bot"

	// Blank-import required drivers.
	_ "mework/libs/server/bus/postgres"
	_ "mework/libs/server/storage/fs"
)

func main() {
	log.Println("Starting mework server...")

	// 1. Load configuration from environment
	cfg, err := hub.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// 2. Run migrations on startup
	log.Println("Running database migrations...")
	if err := store.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatalf("Migration error: %v", err)
	}
	log.Println("Database migrations complete.")

	// 3. Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	dbStore, err := store.NewStore(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatalf("Database connection error: %v", err)
	}
	defer dbStore.Close()
	log.Println("Database connection established.")

	// 4. Initialize server
	srvInstance := hub.NewServer(dbStore.Pool, cfg)
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srvInstance,
		// Bound header-read time to mitigate slowloris. No WriteTimeout: SSE
		// streams are long-lived. IdleTimeout exceeds the SSE heartbeat so kept-
		// alive stream connections are not closed prematurely.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 4a. Start Mezon bot service when credentials are configured.
	var mezonSvc *hub.MezonBotService
	if cfg.MezonAppID != "" && cfg.MezonAPIKey != "" {
		log.Println("Mezon credentials found, starting bot service...")

		// TODO: Wire a real SDK client here once the mezon-go-sdk dependency
		// is vendored into the server module. The nil SDK client produces a
		// no-op bot that logs received messages but does not connect to Mezon
		// or route messages through the channel router. The adapter is still
		// registered so provider.Get("mezon") works for webhook/api paths.
		bot := mezonbot.New(
			mezoncfg.Config{
				AppID:   cfg.MezonAppID,
				APIKey:  cfg.MezonAPIKey,
				BaseURL: cfg.MezonBaseURL,
			},
			nil, // SDK client — replace with real client for production use.
			func(msg mezonbot.Message) {
				log.Printf("Mezon message from %s in channel %s: %.100s", msg.SenderID, msg.ChannelID, msg.Text)
			},
		)
		mezonSvc = hub.SetupMezon(bot)
		mezonCtx, mezonCancel := context.WithCancel(context.Background())
		mezonSvc.Start(mezonCtx)

		// Cancel the Mezon context on shutdown.
		defer mezonCancel()
	} else {
		log.Println("No Mezon credentials configured — skipping Mezon bot service")
	}

	// 5. Graceful shutdown handler
	shutdownErr := make(chan error, 1)
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutdown signal received, shutting down gracefully...")

		// Allow 15 seconds for active connections to finish
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()

		// Stop the Mezon bot service before HTTP to allow pending
		// write-backs to complete.
		if mezonSvc != nil {
			_ = mezonSvc.Stop(shutdownCtx)
		}

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			shutdownErr <- err
			return
		}
		shutdownErr <- nil
	}()

	// 6. Start HTTP server
	log.Printf("Listening on %s", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP server failed: %v", err)
	}

	// 7. Await graceful shutdown completion
	if err := <-shutdownErr; err != nil {
		log.Fatalf("Graceful shutdown failed: %v", err)
	}
	log.Println("Server stopped cleanly.")
}
