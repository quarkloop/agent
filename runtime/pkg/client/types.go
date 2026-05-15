package agentclient

import (
	"encoding/json"
	"strings"
	"time"
)

const defaultRuntimeBasePath = "/api/v1/runtime"

const (
	pathHealth = "/health"
	pathInfo   = "/info"
	pathMode   = "/mode"
	pathStats  = "/stats"
	pathChat   = "/chat"
	pathStop   = "/stop"
)

// HealthResponse is returned by the runtime health endpoint.
type HealthResponse struct {
	AgentID string `json:"agent_id,omitempty"`
	Status  string `json:"status"`
}

// InfoResponse is returned by the runtime info endpoint.
type InfoResponse struct {
	AgentID  string   `json:"agent_id"`
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
	Mode     string   `json:"mode"`
	Tools    []string `json:"tools"`
}

// ModeResponse is returned by runtime mode endpoints.
type ModeResponse struct {
	Mode string `json:"mode"`
}

type setModeRequest struct {
	Mode string `json:"mode"`
}

// StatsResponse is returned by the runtime stats endpoint.
type StatsResponse map[string]any

// ChatRequest is the request body for runtime chat endpoints.
type ChatRequest struct {
	Message    string           `json:"message"`
	SessionKey string           `json:"session_key,omitempty"`
	Stream     bool             `json:"stream,omitempty"`
	Mode       string           `json:"mode,omitempty"`
	Files      []FileAttachment `json:"files,omitempty"`
}

// FileAttachment describes a file uploaded alongside a chat message.
type FileAttachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	Path     string `json:"path,omitempty"`
	Content  []byte `json:"-"`
}

// ChatResponse is returned by chat endpoints.
type ChatResponse struct {
	Reply        string `json:"reply"`
	Mode         string `json:"mode,omitempty"`
	Warning      string `json:"warning,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
}

// PlanStatus represents the status of a plan.
type PlanStatus string

const (
	PlanDraft     PlanStatus = "draft"
	PlanApproved  PlanStatus = "approved"
	PlanRejected  PlanStatus = "rejected"
	PlanExecuting PlanStatus = "executing"
	PlanSucceeded PlanStatus = "succeeded"
	PlanFailed    PlanStatus = "failed"
)

// StepStatus represents the execution state of a single plan step.
type StepStatus string

const (
	StepPending  StepStatus = "pending"
	StepRunning  StepStatus = "running"
	StepComplete StepStatus = "complete"
	StepFailed   StepStatus = "failed"
)

// PlanStep is a single unit of work within an execution plan.
type PlanStep struct {
	ID          string     `json:"id"`
	Agent       string     `json:"agent"`
	Description string     `json:"description"`
	DependsOn   []string   `json:"depends_on"`
	Status      StepStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// PlanResponse represents an execution plan.
type PlanResponse struct {
	Goal      string     `json:"goal"`
	Status    PlanStatus `json:"status"`
	Steps     []PlanStep `json:"steps"`
	Complete  bool       `json:"complete"`
	Summary   string     `json:"summary,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type planActionRequest struct {
	PlanID string `json:"plan_id,omitempty"`
}

// ActivityRecord represents a single runtime activity log entry.
type ActivityRecord struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id,omitempty"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
}

func joinPath(basePath, suffix string) string {
	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		return suffix
	}
	return basePath + suffix
}
