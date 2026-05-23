package channel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// ChannelBus registers, starts, stops, and manages channels.
type ChannelBus struct {
	mu       sync.RWMutex
	channels map[ChannelType][]Channel
}

// NewChannelBus creates a new ChannelBus.
func NewChannelBus() *ChannelBus {
	return &ChannelBus{channels: make(map[ChannelType][]Channel)}
}

// Register adds a channel to the bus.
func (b *ChannelBus) Register(ch Channel) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.channels[ch.Type()] = append(b.channels[ch.Type()], ch)
	slog.Info("channel registered", "type", ch.Type(), "count", len(b.channels[ch.Type()]))
}

// Start starts all registered channels.
func (b *ChannelBus) Start(ctx context.Context) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for channelType, channels := range b.channels {
		for i, ch := range channels {
			if err := ch.Start(ctx); err != nil {
				return fmt.Errorf("start %s[%d]: %w", channelType, i, err)
			}
			slog.Info("channel started", "type", ch.Type(), "index", i)
		}
	}
	return nil
}

// Stop stops all registered channels.
func (b *ChannelBus) Stop(ctx context.Context) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var lastErr error
	for channelType, channels := range b.channels {
		for i, ch := range channels {
			if err := ch.Stop(ctx); err != nil {
				slog.Error("channel stop error", "type", ch.Type(), "index", i, "error", err)
				lastErr = err
			}
		}
		slog.Info("channel group stopped", "type", channelType, "count", len(channels))
	}
	return lastErr
}

// Get returns a registered channel by type.
func (b *ChannelBus) Get(channelType ChannelType) Channel {
	b.mu.RLock()
	defer b.mu.RUnlock()
	channels := b.channels[channelType]
	if len(channels) == 0 {
		return nil
	}
	return channels[0]
}

// ActiveChannels returns info about currently registered channels.
func (b *ChannelBus) ActiveChannels() []ChannelInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]ChannelInfo, 0, len(b.channels))
	for ct, channels := range b.channels {
		for range channels {
			out = append(out, ChannelInfo{Type: ct, Active: true})
		}
	}
	return out
}

// AvailableChannels returns all known channel types with their active status.
func (b *ChannelBus) AvailableChannels() []ChannelInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]ChannelInfo, len(AllChannelTypes))
	for i, ct := range AllChannelTypes {
		active := len(b.channels[ct]) > 0
		out[i] = ChannelInfo{Type: ct, Active: active}
	}
	return out
}
