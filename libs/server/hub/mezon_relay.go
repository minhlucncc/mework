// Package hub — Mezon message relay endpoint for channel-routed sessions.
//
// When a Mezon message arrives, the relay extracts the channel ID and routes
// through the channel router. If the channel has an active session binding,
// the message goes directly to that session's worker. If not, it falls back
// to the orchestrator (legacy path).
package hub

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
)

// channelRouter is the subset of channel.Router that the relay needs.
type channelRouter interface {
	Route(ctx context.Context, providerCode, resourceID, eventType string, payload []byte) error
}

// featureChecker checks whether channel routing is enabled.
type featureChecker interface {
	IsEnabled() bool
}

// mezonRelay is the HTTP handler for POST /api/v1/mezon/messages.
type mezonRelay struct {
	router   channelRouter
	featureC featureChecker
}

// mezonMessage is the payload the worker sends to the relay endpoint.
type mezonMessage struct {
	ChannelID string `json:"channel_id"`
	ClanID    string `json:"clan_id,omitempty"`
	SenderID  string `json:"sender_id"`
	Text      string `json:"text"`
	MessageID string `json:"message_id,omitempty"`
	BotKeyID  string `json:"bot_key_id,omitempty"`
	BotToken  string `json:"bot_token,omitempty"`
}

func newMezonRelay(router channelRouter, fc featureChecker) *mezonRelay {
	return &mezonRelay{router: router, featureC: fc}
}

func (r *mezonRelay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// 1. Decode the incoming message.
	var msg mezonMessage
	if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if msg.ChannelID == "" || msg.Text == "" {
		http.Error(w, "channel_id and text are required", http.StatusBadRequest)
		return
	}

	// 2. If channel routing is enabled, try to route to a session.
	if r.router != nil && r.featureC != nil && r.featureC.IsEnabled() {
		payload, _ := json.Marshal(msg)
		err := r.router.Route(req.Context(), "mezon", msg.ChannelID, "dispatch", payload)
		if err == nil {
			log.Printf("mezon-relay: routed message from channel %s to session", msg.ChannelID)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status":     "routed",
				"channel_id": msg.ChannelID,
			})
			return
		}
		log.Printf("mezon-relay: channel routing failed for %s, falling back: %v", msg.ChannelID, err)
	}

	// 3. Fallback: route to orchestrator (legacy).
	log.Printf("mezon-relay: message from channel %s routed to orchestrator (fallback)", msg.ChannelID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "orchestrator",
		"channel_id": msg.ChannelID,
	})
}
