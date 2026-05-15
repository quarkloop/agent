package api

import (
	"encoding/json"
	"time"

	"github.com/quarkloop/runtime/pkg/activity"
	"github.com/quarkloop/runtime/pkg/message"
)

type errorResponse struct {
	Error string `json:"error"`
}

type statusResponse struct {
	Status string `json:"status"`
}

type sendMessageRequest struct {
	Content string `json:"content"`
}

func mapSendMessageRequest(sessionID string, req sendMessageRequest) message.PostRequest {
	return message.PostRequest{
		SessionID: sessionID,
		Content:   req.Content,
	}
}

type messageResponse struct {
	ID        string `json:"id,omitempty"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

func mapMessageResponses(messages []message.Message) []messageResponse {
	out := make([]messageResponse, 0, len(messages))
	for _, msg := range messages {
		out = append(out, messageResponse{
			ID:        msg.ID,
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		})
	}
	return out
}

type agentInfoResponse struct {
	ID           string   `json:"id"`
	Sessions     int      `json:"sessions"`
	WorkStatus   string   `json:"work_status"`
	DefaultModel string   `json:"default_model"`
	Models       []string `json:"models"`
	Channels     any      `json:"channels,omitempty"`
}

type channelsResponse struct {
	Active    any `json:"active"`
	Available any `json:"available"`
}

type activityResponse struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id,omitempty"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
}

func mapActivityResponse(record activity.Record) activityResponse {
	return activityResponse{
		ID:        record.ID,
		SessionID: record.SessionID,
		Type:      record.Type,
		Timestamp: record.Timestamp,
		Data:      append(json.RawMessage(nil), record.Data...),
	}
}

type planStepResponse struct {
	ID          string   `json:"id"`
	Agent       string   `json:"agent"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on"`
	Status      string   `json:"status"`
	Result      string   `json:"result,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type planResponse struct {
	Goal      string             `json:"goal"`
	Status    string             `json:"status"`
	Steps     []planStepResponse `json:"steps"`
	Complete  bool               `json:"complete"`
	Summary   string             `json:"summary,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}
