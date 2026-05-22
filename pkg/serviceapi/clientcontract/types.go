package clientcontract

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type RequestEnvelope struct {
	Version     string          `json:"version"`
	RequestID   string          `json:"request_id"`
	SpaceID     string          `json:"space_id,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	Actor       string          `json:"actor,omitempty"`
	TraceParent string          `json:"traceparent,omitempty"`
	Payload     json.RawMessage `json:"payload"`
}

type ResponseEnvelope struct {
	Version     string          `json:"version"`
	RequestID   string          `json:"request_id"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Error       *ErrorPayload   `json:"error,omitempty"`
	TraceParent string          `json:"traceparent,omitempty"`
}

type ErrorPayload struct {
	Category string `json:"category"`
	Message  string `json:"message"`
}

type SpaceInfo struct {
	Name       string    `json:"name"`
	Version    string    `json:"version,omitempty"`
	WorkingDir string    `json:"working_dir,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CreateSpaceRequest struct {
	Name       string `json:"name"`
	Quarkfile  []byte `json:"quarkfile"`
	WorkingDir string `json:"working_dir"`
}

type GetSpaceRequest struct {
	Name string `json:"name"`
}

type ListSpacesResponse struct {
	Spaces []SpaceInfo `json:"spaces"`
}

type SessionType string

const (
	SessionTypeChat SessionType = "chat"
	SessionTypeTask SessionType = "task"
)

type SessionInfo struct {
	ID        string      `json:"id"`
	Type      SessionType `json:"type"`
	Title     string      `json:"title,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type CreateSessionRequest struct {
	SpaceID string      `json:"space_id"`
	Type    SessionType `json:"type"`
	Title   string      `json:"title,omitempty"`
}

type SessionRefRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
}

type ListSessionsRequest struct {
	SpaceID string `json:"space_id"`
}

type ListSessionsResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}

type SendMessageRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

type SessionEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	RunID     string          `json:"run_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type ListServicesRequest struct {
	SpaceID string `json:"space_id"`
}

type InspectRuntimeRequest struct {
	SpaceID   string `json:"space_id,omitempty"`
	RuntimeID string `json:"runtime_id,omitempty"`
}

type ArtifactRef struct {
	SpaceID    string `json:"space_id"`
	ArtifactID string `json:"artifact_id"`
}

func NewRequest(requestID, spaceID string, payload any) (RequestEnvelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestEnvelope{}, fmt.Errorf("marshal payload: %w", err)
	}
	req := RequestEnvelope{
		Version:   Version,
		RequestID: strings.TrimSpace(requestID),
		SpaceID:   strings.TrimSpace(spaceID),
		Payload:   data,
	}
	if err := req.Validate(); err != nil {
		return RequestEnvelope{}, err
	}
	return req, nil
}

func (e RequestEnvelope) Validate() error {
	if e.Version != Version {
		return fmt.Errorf("unsupported client request version %q", e.Version)
	}
	if strings.TrimSpace(e.RequestID) == "" {
		return fmt.Errorf("request_id is required")
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("payload is required")
	}
	if !json.Valid(e.Payload) {
		return fmt.Errorf("payload must be valid JSON")
	}
	return nil
}

func (e RequestEnvelope) DecodePayload(out any) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := json.Unmarshal(e.Payload, out); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	return nil
}

func (e RequestEnvelope) Clone() RequestEnvelope {
	out := e
	out.Payload = append(json.RawMessage(nil), e.Payload...)
	return out
}

func OK(requestID string, payload any) (ResponseEnvelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return ResponseEnvelope{}, fmt.Errorf("marshal response payload: %w", err)
	}
	resp := ResponseEnvelope{
		Version:   Version,
		RequestID: strings.TrimSpace(requestID),
		Status:    "ok",
		Payload:   data,
	}
	return resp, resp.Validate()
}

func Error(requestID, category, message string) ResponseEnvelope {
	return ResponseEnvelope{
		Version:   Version,
		RequestID: strings.TrimSpace(requestID),
		Status:    "error",
		Error: &ErrorPayload{
			Category: strings.TrimSpace(category),
			Message:  strings.TrimSpace(message),
		},
	}
}

func (e ResponseEnvelope) Validate() error {
	if e.Version != Version {
		return fmt.Errorf("unsupported client response version %q", e.Version)
	}
	if strings.TrimSpace(e.RequestID) == "" {
		return fmt.Errorf("request_id is required")
	}
	switch e.Status {
	case "ok":
		if e.Error != nil {
			return fmt.Errorf("ok response must not include error")
		}
		if len(e.Payload) > 0 && !json.Valid(e.Payload) {
			return fmt.Errorf("payload must be valid JSON")
		}
	case "error":
		if e.Error == nil {
			return fmt.Errorf("error response requires error payload")
		}
	default:
		return fmt.Errorf("invalid response status %q", e.Status)
	}
	return nil
}

func (e ResponseEnvelope) DecodePayload(out any) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Status == "error" {
		return fmt.Errorf("%s: %s", e.Error.Category, e.Error.Message)
	}
	if len(e.Payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(e.Payload, out); err != nil {
		return fmt.Errorf("decode response payload: %w", err)
	}
	return nil
}

func (e ResponseEnvelope) Clone() ResponseEnvelope {
	out := e
	out.Payload = append(json.RawMessage(nil), e.Payload...)
	if e.Error != nil {
		errPayload := *e.Error
		out.Error = &errPayload
	}
	return out
}
