package agent

import (
	"context"
	"fmt"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/runtime/pkg/activity"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/modelservice"
)

func (a *Agent) instrumentResponse(ctx context.Context, sessionID string, downstream chan message.StreamMessage) (chan message.StreamMessage, func()) {
	upstream := make(chan message.StreamMessage, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for msg := range upstream {
			a.recordStreamActivity(sessionID, msg)
			if !message.Emit(ctx, downstream, msg) {
				return
			}
		}
	}()
	return upstream, func() {
		close(upstream)
		<-done
	}
}

func (a *Agent) recordStreamActivity(sessionID string, msg message.StreamMessage) {
	if a.Activity == nil {
		return
	}
	switch msg.Type {
	case "tool_start", "tool_result":
		a.addActivity(sessionID, msg.Type, msg.Data)
	case "error":
		a.addActivity(sessionID, "message.error", map[string]any{"error": fmt.Sprint(msg.Data)})
	}
}

func (a *Agent) emitMessageError(ctx context.Context, sessionID string, response chan message.StreamMessage, err error) {
	payload := boundary.StreamPayload(err, boundary.Runtime, "message")
	if runID := modelservice.RunID(ctx); runID != "" {
		payload["run_id"] = runID
	}
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	if messageText, ok := payload["message"].(string); ok {
		payload["message"] = fmt.Sprintf("Agent Error: %s", messageText)
	}
	if a.Activity != nil {
		a.addActivity(sessionID, "message.error", payload)
	}
	message.Emit(ctx, response, message.StreamMessage{Type: "error", Data: payload})
}

func (a *Agent) recordModelUsage(ctx context.Context, usage modelservice.Usage) {
	sessionID := usage.SessionID
	if sessionID == "" {
		sessionID = modelservice.SessionID(ctx)
		usage.SessionID = sessionID
	}
	if a.Activity != nil {
		a.addActivity(sessionID, "model.usage", usage)
	}
}

func (a *Agent) addActivity(sessionID, typ string, data any) activity.Record {
	if a.Activity == nil {
		return activity.Record{}
	}
	record := a.Activity.Add(sessionID, typ, data)
	if a.core != nil {
		a.core.Record(record)
	}
	return record
}
