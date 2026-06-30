package hub

import (
	mezonadapter "mework/libs/server/provider/mezon"
	mezonbot "mework/libs/server/provider/mezon/bot"
)

// SetupMezon initializes the Mezon provider in the hub server. It creates the
// adapter wrapping the bot, registers it with the global provider registry, and
// returns a started MezonBotService for lifecycle management.
//
// Callers (typically apps/mework-server/main.go) create the mezonbot.Bot with
// an SDK client and a dispatch handler that routes messages through the channel
// router, then pass it to SetupMezon. The returned MezonBotService should be
// stopped during graceful shutdown.
func SetupMezon(bot *mezonbot.Bot) *MezonBotService {
	mezonadapter.RegisterAdapter(bot)
	return NewMezonBotService(bot)
}
