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

type DeleteSpaceRequest struct {
	Name string `json:"name"`
}

type UpdateSpaceRequest struct {
	Name      string `json:"name"`
	Quarkfile []byte `json:"quarkfile"`
}

type QuarkfileRequest struct {
	Name string `json:"name"`
}

type QuarkfileResponse struct {
	Name      string    `json:"name"`
	Version   string    `json:"version,omitempty"`
	Quarkfile []byte    `json:"quarkfile"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DoctorRequest struct {
	Name string `json:"name"`
}

type DoctorIssue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type DoctorResponse struct {
	OK     bool          `json:"ok"`
	Issues []DoctorIssue `json:"issues"`
}

type SpaceCredentialRequest struct {
	SpaceID string `json:"space_id"`
}

type SpaceCredentialResponse struct {
	Credential NATSCredential `json:"credential"`
}

type ListSpacesResponse struct {
	Spaces []SpaceInfo `json:"spaces"`
}

type SessionType string

const (
	SessionTypeMain     SessionType = "main"
	SessionTypeChat     SessionType = "chat"
	SessionTypeTask     SessionType = "task"
	SessionTypeSubAgent SessionType = "subagent"
	SessionTypeCron     SessionType = "cron"
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

type SessionCredentialRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
}

type NATSCredential struct {
	URL       string `json:"url,omitempty"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Account   string `json:"account"`
	Role      string `json:"role"`
	SpaceID   string `json:"space_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
}

type SessionCredentialResponse struct {
	Credential NATSCredential `json:"credential"`
}

type ListSessionsRequest struct {
	SpaceID string `json:"space_id"`
}

type ListSessionsResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}

type KBRefRequest struct {
	SpaceID   string `json:"space_id"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
}

type KBSetRequest struct {
	SpaceID   string `json:"space_id"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Value     []byte `json:"value"`
}

type KBListRequest struct {
	SpaceID   string `json:"space_id"`
	Namespace string `json:"namespace"`
}

type KBValueResponse struct {
	Value []byte `json:"value"`
}

type KBListResponse struct {
	Keys []string `json:"keys"`
}

type SendMessageRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

type SendMessageResponse struct {
	SessionID string `json:"session_id"`
	Accepted  bool   `json:"accepted"`
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

type InspectServiceRequest struct {
	SpaceID string `json:"space_id"`
	Service string `json:"service"`
}

type ServiceStatus string

const (
	ServiceStatusStarting     ServiceStatus = "starting"
	ServiceStatusReady        ServiceStatus = "ready"
	ServiceStatusUnavailable  ServiceStatus = "unavailable"
	ServiceStatusMissing      ServiceStatus = "missing"
	ServiceStatusUnconfigured ServiceStatus = "unconfigured"
	ServiceStatusStopping     ServiceStatus = "stopping"
	ServiceStatusStopped      ServiceStatus = "stopped"
)

type ServiceFunctionInfo struct {
	Name        string `json:"name"`
	Service     string `json:"service"`
	Method      string `json:"method"`
	Request     string `json:"request"`
	Response    string `json:"response"`
	Description string `json:"description"`
	RiskLevel   string `json:"risk_level,omitempty"`
	Approval    bool   `json:"approval_required,omitempty"`
	Idempotent  bool   `json:"idempotent,omitempty"`
}

type ServiceInfo struct {
	Name          string                `json:"name"`
	Type          string                `json:"type"`
	Version       string                `json:"version"`
	Mode          string                `json:"mode"`
	Description   string                `json:"description"`
	Status        ServiceStatus         `json:"status"`
	PID           int                   `json:"pid,omitempty"`
	Endpoint      string                `json:"endpoint,omitempty"`
	LogPath       string                `json:"log_path,omitempty"`
	StartedAt     *time.Time            `json:"started_at,omitempty"`
	AddressEnv    string                `json:"address_env,omitempty"`
	HealthService string                `json:"health_service,omitempty"`
	MinVersion    string                `json:"min_version,omitempty"`
	FunctionCount int                   `json:"function_count"`
	Functions     []ServiceFunctionInfo `json:"functions,omitempty"`
	Diagnostics   []string              `json:"diagnostics,omitempty"`
}

type ListServicesResponse struct {
	Services []ServiceInfo `json:"services"`
}

type ServiceDoctorResponse struct {
	Services []ServiceInfo `json:"services"`
	Issues   []string      `json:"issues,omitempty"`
}

type PluginInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Type        string `json:"type"`
	Mode        string `json:"mode"`
	Description string `json:"description"`
}

type ListPluginsRequest struct {
	SpaceID    string `json:"space_id"`
	TypeFilter string `json:"type_filter,omitempty"`
}

type ListPluginsResponse struct {
	Plugins []PluginInfo `json:"plugins"`
}

type PluginRefRequest struct {
	SpaceID string `json:"space_id"`
	Plugin  string `json:"plugin"`
}

type InstallPluginRequest struct {
	SpaceID string `json:"space_id"`
	Ref     string `json:"ref"`
}

type InstallPluginResponse struct {
	Plugin PluginInfo `json:"plugin"`
}

type SearchPluginsRequest struct {
	SpaceID string `json:"space_id"`
	Query   string `json:"query"`
}

type PluginSearchResult struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Author      string `json:"author"`
}

type SearchPluginsResponse struct {
	Results []PluginSearchResult `json:"results"`
}

type HubPluginInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	License     string   `json:"license"`
	Repository  string   `json:"repository"`
	Downloads   int      `json:"downloads"`
	Versions    []string `json:"versions"`
}

type InspectRuntimeRequest struct {
	SpaceID   string `json:"space_id,omitempty"`
	RuntimeID string `json:"runtime_id,omitempty"`
}

type ArtifactRef struct {
	SpaceID    string `json:"space_id"`
	ArtifactID string `json:"artifact_id"`
}

type RuntimePlanRequest struct {
	SpaceID string `json:"space_id"`
	PlanID  string `json:"plan_id,omitempty"`
}

type RuntimePlanStep struct {
	ID          string `json:"id"`
	Agent       string `json:"agent"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Result      string `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
}

type RuntimePlanResponse struct {
	Goal      string            `json:"goal"`
	Status    string            `json:"status"`
	Steps     []RuntimePlanStep `json:"steps"`
	Complete  bool              `json:"complete"`
	Summary   string            `json:"summary,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type RuntimeActivityListRequest struct {
	SpaceID string `json:"space_id"`
	Limit   int    `json:"limit,omitempty"`
}

type RuntimeActivityRecord struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id,omitempty"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type RuntimeActivityListResponse struct {
	Records []RuntimeActivityRecord `json:"records"`
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
