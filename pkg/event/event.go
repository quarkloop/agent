// Package event defines the canonical event types and constants shared across
// supervisor, runtime, and CLI packages.
package event

import (
	"encoding/json"
	"time"
)

// Kind is the typed event discriminator.
type Kind string

// EventKind constants published by the supervisor on the space event stream.
const (
	SessionCreated     Kind = "session.created"
	SessionDeleted     Kind = "session.deleted"
	SpaceConfigUpdated Kind = "space.config.updated"
	PluginInstalled    Kind = "plugin.installed"
	PluginRemoved      Kind = "plugin.removed"
	RuntimeShutdown    Kind = "runtime.shutdown"
)

// Event is the wire format for a supervisor → agent signal.
type Event struct {
	Kind    Kind            `json:"kind"`
	Space   string          `json:"space"`
	Time    time.Time       `json:"time"`
	Payload json.RawMessage `json:"payload,omitempty"`
}
