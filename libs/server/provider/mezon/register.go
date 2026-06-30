package mezon

import "mework/libs/server/provider"

// RegisterAdapter registers the MezonAdapter with the global provider registry.
func RegisterAdapter(bot BotSender) {
	provider.Register(NewMezonAdapter(bot))
}
