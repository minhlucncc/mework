// Package bus defines the message-bus interface for server-internal
// event publishing and subscription. It wraps the Broker port from shared/ports.
//
// This is a stub — the full implementation lands in a downstream change.
package bus

import (
	"context"

	"mework/shared/core"
)

// Bus is the server's message bus interface.
type Bus interface {
	// Publish sends a message on a topic.
	Publish(ctx context.Context, topic core.Topic, msg core.Message) error
	// Subscribe registers a handler for a topic.
	Subscribe(ctx context.Context, topic core.Topic, handler func(core.Message) error) error
}
