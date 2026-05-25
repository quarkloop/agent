// Package events is the supervisor's fan-out bus for space-scoped signals
// consumed by agents (primarily over SSE).
package events

import (
	"encoding/json"
	"sync"
	"time"

	event "github.com/quarkloop/pkg/event"
)

// Kind and constants are defined in pkg/event.
type Kind = event.Kind

const (
	SessionCreated     = event.SessionCreated
	SessionDeleted     = event.SessionDeleted
	SpaceConfigUpdated = event.SpaceConfigUpdated
	PluginInstalled    = event.PluginInstalled
	PluginRemoved      = event.PluginRemoved
	RuntimeShutdown    = event.RuntimeShutdown
)

// Bus is an in-memory fan-out of events scoped by space.
type Bus struct {
	mu   sync.RWMutex
	subs map[string]map[chan event.Event]struct{} // space → subscribers
}

// NewBus returns a fresh event bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[string]map[chan event.Event]struct{})}
}

// Publish delivers e to every subscriber of e.Space. Slow subscribers are
// dropped; publish never blocks.
func (b *Bus) Publish(e event.Event) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	// Deep-copy the payload to avoid sharing the underlying array with the caller.
	if len(e.Payload) > 0 {
		payloadCopy := make(json.RawMessage, len(e.Payload))
		copy(payloadCopy, e.Payload)
		e.Payload = payloadCopy
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs[e.Space] {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe returns a channel receiving events for space. Call the returned
// function to unsubscribe.
func (b *Bus) Subscribe(space string) (<-chan event.Event, func()) {
	ch := make(chan event.Event, 32)
	b.mu.Lock()
	if b.subs[space] == nil {
		b.subs[space] = make(map[chan event.Event]struct{})
	}
	b.subs[space][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs[space], ch)
		b.mu.Unlock()
		close(ch)
	}
}

// Helpers to build typed payloads.

func SessionPayload(id string, kind string, title string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"id": id, "type": kind, "title": title})
	return b
}

func PluginPayload(name, typ string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"name": name, "type": typ})
	return b
}
