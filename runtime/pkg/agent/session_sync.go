package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	event "github.com/quarkloop/pkg/event"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

func (a *Agent) Supervisor() *supclient.Client {
	return a.supervisorClient
}

func (a *Agent) HasSupervisor() bool {
	return a.supervisorClient != nil
}

// subscribeSupervisorEvents is a transitional legacy session-mirroring path.
// NATS-native session routing removes it in the runtime execution-plane task.
func (a *Agent) subscribeSupervisorEvents(ctx context.Context) {
	if a.supervisorClient == nil || a.Space == "" {
		slog.Info("supervisor event stream disabled", "client", a.supervisorClient != nil, "space", a.Space)
		return
	}
	slog.Info("subscribing to supervisor events", "space", a.Space)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := a.supervisorClient.StreamEventsWithReady(ctx, a.Space,
			func() {
				slog.Info("supervisor event stream ready", "space", a.Space)
				a.syncSupervisorSessions(ctx)
			},
			func(ev event.Event) { a.applyEvent(ev) },
		)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Error("supervisor event stream error, retrying", "error", err, "retry_in", backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (a *Agent) syncSupervisorSessions(ctx context.Context) {
	if a.supervisorClient == nil || a.Space == "" {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sessions, err := a.supervisorClient.ListSessions(callCtx, a.Space)
	if err != nil {
		slog.Warn("sync supervisor sessions failed", "space", a.Space, "error", err)
		return
	}
	for _, sess := range sessions {
		if sess.ID != "" {
			a.Sessions.GetOrCreate(sess.ID, string(sess.Type), sess.Title)
		}
	}
	slog.Info("synced supervisor sessions", "space", a.Space, "count", len(sessions))
}

func (a *Agent) applyEvent(ev event.Event) {
	switch ev.Kind {
	case event.SessionCreated:
		var payload struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(ev.Payload, &payload); err != nil || payload.ID == "" {
			return
		}
		a.Sessions.GetOrCreate(payload.ID, payload.Type, payload.Title)
		slog.Info("session created", "id", payload.ID, "type", payload.Type)
	case event.SessionDeleted:
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(ev.Payload, &payload); err != nil || payload.ID == "" {
			return
		}
		a.Sessions.Delete(payload.ID)
		slog.Info("session deleted", "id", payload.ID)
	}
}
