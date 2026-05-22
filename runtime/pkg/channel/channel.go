// Package channel provides the channel interface and channel bus.
//
// A channel is a communication transport (HTTP server, Telegram bot, etc.)
// that owns its own server/connection lifecycle.
package channel

import "context"

type ChannelType string

const (
	WebChannelType      ChannelType = "web"
	TelegramChannelType ChannelType = "telegram"
	NATSChannelType     ChannelType = "nats"
)

// Channel is a communication transport that can be started and stopped.
type Channel interface {
	Type() ChannelType
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
