package api

import (
	"encoding/json"
	"time"

	event "github.com/quarkloop/pkg/event"
)

// --- Agent runtime types (returned by the agent's own HTTP API) ---

// HealthResponse is returned by the agent health endpoint.
type HealthResponse struct {
	AgentID string `json:"agent_id,omitempty"`
	Status  string `json:"status"`
}

// InfoResponse is returned by the agent info endpoint.
type InfoResponse struct {
	AgentID  string   `json:"agent_id"`
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
	Mode     string   `json:"mode"`
	Tools    []string `json:"tools"`
}

// ModeResponse is returned by mode endpoints.
type ModeResponse struct {
	Mode string `json:"mode"`
}

// SetModeRequest is the request body for setting mode.
type SetModeRequest struct {
	Mode string `json:"mode"`
}

// StatsResponse is returned by the stats endpoint.
type StatsResponse map[string]any

// AgentChatRequest is the request body for chat endpoints.
type AgentChatRequest struct {
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

// PlanActionRequest is the request body for plan approve/reject.
type PlanActionRequest struct {
	PlanID string `json:"plan_id,omitempty"`
}

// ActivityRecord represents a single activity log entry.
type ActivityRecord struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id,omitempty"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// SessionType is the type of a session.
type SessionType string

const (
	SessionTypeMain     SessionType = "main"
	SessionTypeChat     SessionType = "chat"
	SessionTypeSubAgent SessionType = "subagent"
	SessionTypeCron     SessionType = "cron"
)

// Session is the supervisor-owned record for a conversation. Sessions are
// identified by a supervisor-generated id and scoped to a single space.
type Session struct {
	ID        string      `json:"id"`
	Space     string      `json:"space"`
	Type      SessionType `json:"type"`
	Title     string      `json:"title,omitempty"`
	Status    string      `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// CreateSessionRequest is the body for POST /v1/spaces/{name}/sessions.
type CreateSessionRequest struct {
	Type  SessionType `json:"type,omitempty"`
	Title string      `json:"title,omitempty"`
}

// Event is the wire format for a supervisor → agent signal. The supervisor
// publishes events on its space-scoped SSE stream and agents consume them to
// stay in sync with supervisor state (sessions, plugins, quarkfile, etc).
// It is an alias for the canonical event.Event from pkg/event.
type Event = event.Event

// Event kinds are defined in pkg/event. Re-export for API backwards compatibility.
const (
	EventSessionCreated   = event.SessionCreated
	EventSessionDeleted   = event.SessionDeleted
	EventQuarkfileUpdated = event.QuarkfileUpdated
	EventPluginInstalled  = event.PluginInstalled
	EventPluginRemoved    = event.PluginRemoved
	EventRuntimeShutdown  = event.RuntimeShutdown
)

// BudgetResponse is returned by budget endpoints.
type BudgetResponse struct {
	TotalBudget      int     `json:"total_budget"`
	UsedTokens       int     `json:"used_tokens"`
	AvailableTokens  int     `json:"available_tokens"`
	UsagePct         float64 `json:"usage_pct"`
	AtSoftLimit      bool    `json:"at_soft_limit"`
	AtHardLimit      bool    `json:"at_hard_limit"`
	CompactionNeeded bool    `json:"compaction_needed"`
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error string `json:"error"`
}

// --- Space (data) types ---

// SpaceInfo identifies a space and exposes non-sensitive metadata. A space is
// a supervisor-owned data namespace keyed by Name (from Quarkfile meta.name).
// WorkingDir is the user workspace where the agent is launched.
type SpaceInfo struct {
	Name       string    `json:"name"`
	Version    string    `json:"version,omitempty"`
	WorkingDir string    `json:"working_dir,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateSpaceRequest is the body for POST /v1/spaces. The CLI sends the
// space name, Quarkfile contents, and working directory path.
// The supervisor creates the space in its storage and writes the Quarkfile
// to the working directory.
type CreateSpaceRequest struct {
	Name       string `json:"name"`
	Quarkfile  []byte `json:"quarkfile"`
	WorkingDir string `json:"working_dir"`
}

// UpdateQuarkfileRequest is the body for PUT /v1/spaces/{name}/quarkfile.
type UpdateQuarkfileRequest struct {
	Quarkfile []byte `json:"quarkfile"`
}

// QuarkfileResponse is returned when fetching a space's stored Quarkfile.
type QuarkfileResponse struct {
	Name string `json:"name"`
	// Version is meta.version from this Quarkfile.
	Version   string    `json:"version,omitempty"`
	Quarkfile []byte    `json:"quarkfile"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DoctorIssue describes a single problem discovered by the doctor check.
type DoctorIssue struct {
	Severity string `json:"severity"` // "error" | "warning"
	Message  string `json:"message"`
}

// DoctorResponse is returned by POST /v1/spaces/{name}/doctor.
type DoctorResponse struct {
	OK     bool          `json:"ok"`
	Issues []DoctorIssue `json:"issues"`
}

// --- Runtime types ---

// RuntimeStatus is the lifecycle state of a running runtime.
type RuntimeStatus string

const (
	RuntimeStarting RuntimeStatus = "starting"
	RuntimeRunning  RuntimeStatus = "running"
	RuntimeStopping RuntimeStatus = "stopping"
	RuntimeStopped  RuntimeStatus = "stopped"
)

// AgentInfo describes a supervisor-managed agent process.
type RuntimeInfo struct {
	ID         string        `json:"id"`
	Space      string        `json:"space"`
	WorkingDir string        `json:"working_dir"`
	Status     RuntimeStatus `json:"status"`
	PID        int           `json:"pid,omitempty"`
	Port       int           `json:"port,omitempty"`
	StartedAt  time.Time     `json:"started_at,omitempty"`
	Uptime     string        `json:"uptime,omitempty"`
}

// URL returns the runtime's HTTP base URL or empty when not running.
func (a RuntimeInfo) URL() string {
	if a.Port == 0 {
		return ""
	}
	return "http://127.0.0.1:" + itoa(a.Port)
}

// StartRuntimeRequest is the body for POST /v1/agents.
type StartRuntimeRequest struct {
	Space string `json:"space"`
	Port  int    `json:"port,omitempty"`
}

// --- Plugin management types ---

// PluginInfo describes an installed plugin in a space.
type PluginInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Type        string `json:"type"`
	Mode        string `json:"mode"`
	Description string `json:"description"`
}

// ListPluginsResponse is returned by GET /v1/spaces/{name}/plugins.
type ListPluginsResponse struct {
	Plugins []PluginInfo `json:"plugins"`
}

// InstallPluginRequest is the body for POST /v1/spaces/{name}/plugins.
type InstallPluginRequest struct {
	Ref string `json:"ref"`
}

// InstallPluginResponse is returned by POST /v1/spaces/{name}/plugins.
type InstallPluginResponse struct {
	Plugin PluginInfo `json:"plugin"`
}

// PluginSearchResult is a single result from hub plugin search.
type PluginSearchResult struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Author      string `json:"author"`
}

// SearchPluginsResponse is returned by GET /v1/spaces/{name}/plugins/search.
type SearchPluginsResponse struct {
	Results []PluginSearchResult `json:"results"`
}

// HubPluginInfo is the detailed hub response for a plugin.
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

// --- Service manager types ---

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

type ServiceLogsResponse struct {
	Name    string `json:"name"`
	LogPath string `json:"log_path,omitempty"`
	Content string `json:"content,omitempty"`
	Message string `json:"message,omitempty"`
}

type ServiceLifecycleResponse struct {
	Service ServiceInfo `json:"service"`
	Message string      `json:"message,omitempty"`
}

type ServiceRestartResponse = ServiceLifecycleResponse

type ServiceDoctorResponse struct {
	Services []ServiceInfo `json:"services"`
	Issues   []string      `json:"issues,omitempty"`
}

// --- KB types ---

// KBSetRequest is the body for PUT /v1/spaces/{name}/kb/{namespace}/{key}.
type KBSetRequest struct {
	Value []byte `json:"value"`
}

// KBValueResponse is returned by GET /v1/spaces/{name}/kb/{namespace}/{key}.
type KBValueResponse struct {
	Value []byte `json:"value"`
}

// KBListResponse is returned by GET /v1/spaces/{name}/kb/{namespace}.
type KBListResponse struct {
	Keys []string `json:"keys"`
}

// itoa avoids importing strconv at package top-level just for AgentInfo.URL.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
