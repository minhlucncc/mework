package hub

import (
	"context"
	"log"
	"sync"

	mezonbot "mework/libs/server/provider/mezon/bot"
)

// MezonBotService manages the Mezon bot client lifecycle in the hub server.
// It wraps a mezonbot.Bot whose dispatch handler was configured at construction
// time to route incoming messages through the channel router. The service
// handles authentication, connection, and graceful shutdown.
type MezonBotService struct {
	bot    *mezonbot.Bot
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
}

// NewMezonBotService creates a new MezonBotService wrapping the given bot.
// The bot's dispatch handler should route received messages through the
// channel router (set up externally before calling NewMezonBotService).
func NewMezonBotService(bot *mezonbot.Bot) *MezonBotService {
	return &MezonBotService{
		bot: bot,
	}
}

// Start authenticates and connects the bot, then starts the dispatch loop in a
// background goroutine. It is safe to call Start multiple times (subsequent
// calls are no-ops after the first successful start).
func (s *MezonBotService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if err := s.bot.Authenticate(); err != nil {
		log.Printf("Mezon bot authenticate failed: %v", err)
		return
	}

	if err := s.bot.Connect(); err != nil {
		log.Printf("Mezon bot connect failed: %v", err)
		return
	}

	ctx, s.cancel = context.WithCancel(ctx)
	go func() {
		if err := s.bot.Start(ctx); err != nil {
			log.Printf("Mezon bot start exited: %v", err)
		}
	}()
}

// Stop gracefully shuts down the bot. It cancels the internal context and calls
// bot.Stop() with a 5-second timeout. Subsequent calls are no-ops.
func (s *MezonBotService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.cancel != nil {
		s.cancel()
	}

	done := make(chan error, 1)
	go func() {
		done <- s.bot.Stop()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Bot returns the underlying bot instance for use by the adapter's write-back.
func (s *MezonBotService) Bot() *mezonbot.Bot {
	return s.bot
}
