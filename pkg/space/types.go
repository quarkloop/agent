package space

// Model specifies the model provider and model name selected for a space.
type Model struct {
	Provider string   `json:"provider"`
	Name     string   `json:"name"`
	Env      []string `json:"env,omitempty"`
}

func (m Model) IsZero() bool {
	return m.Provider == "" && m.Name == "" && len(m.Env) == 0
}

type RoutingSection struct {
	Rules    []RoutingRuleEntry `json:"rules,omitempty"`
	Fallback []ModelRef         `json:"fallback,omitempty"`
}

type RoutingRuleEntry struct {
	Match    string `json:"match"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ModelRef struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type PluginRef struct {
	Ref    string         `json:"ref"`
	Config map[string]any `json:"config,omitempty"`
}

// AgentRef declares space-level narrowing overrides over an installed agent
// profile. Installed plugins remain the source of profile defaults.
type AgentRef struct {
	Profile  string                `json:"profile"`
	Enabled  *bool                 `json:"enabled,omitempty"`
	Model    AgentModelOverride    `json:"model,omitempty"`
	Services []string              `json:"services,omitempty"`
	Tools    []string              `json:"tools,omitempty"`
	Approval AgentApprovalOverride `json:"approval,omitempty"`
	Memory   AgentMemoryOverride   `json:"memory,omitempty"`
}

type AgentModelOverride struct {
	Provider string   `json:"provider,omitempty"`
	Name     string   `json:"name,omitempty"`
	Env      []string `json:"env,omitempty"`
}

func (m AgentModelOverride) IsZero() bool {
	return m.Provider == "" && m.Name == "" && len(m.Env) == 0
}

type AgentApprovalOverride struct {
	Policy      string   `json:"policy,omitempty"`
	RequiredFor []string `json:"required_for,omitempty"`
}

type AgentMemoryOverride struct {
	Scope       string   `json:"scope,omitempty"`
	Collections []string `json:"collections,omitempty"`
}

// ServiceRef records a space-selected service plugin reference. Supervisor
// resolves concrete service catalog data before runtime consumes it.
type ServiceRef struct {
	Name   string         `json:"name"`
	Ref    string         `json:"ref,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

type Permissions struct {
	Filesystem FilesystemPermissions `json:"filesystem,omitempty"`
	Network    NetworkPermissions    `json:"network,omitempty"`
	Tools      ToolPermissions       `json:"tools,omitempty"`
	Audit      AuditPermissions      `json:"audit,omitempty"`
}

type FilesystemPermissions struct {
	AllowedPaths []string `json:"allowed_paths,omitempty"`
	ReadOnly     []string `json:"read_only,omitempty"`
}

type NetworkPermissions struct {
	AllowedHosts []string `json:"allowed_hosts,omitempty"`
	Deny         []string `json:"deny,omitempty"`
}

type ToolPermissions struct {
	Allowed []string `json:"allowed,omitempty"`
	Denied  []string `json:"denied,omitempty"`
}

type AuditPermissions struct {
	LogToolCalls    bool `json:"log_tool_calls"`
	LogLLMResponses bool `json:"log_llm_responses"`
	RetentionDays   int  `json:"retention_days,omitempty"`
}

type Capabilities struct {
	SpawnAgents    bool   `json:"spawn_agents"`
	MaxWorkers     int    `json:"max_workers,omitempty"`
	CreatePlans    bool   `json:"create_plans"`
	ApprovalPolicy string `json:"approval_policy,omitempty"`
}

type Gateway struct {
	TokenBudgetPerHour int `json:"token_budget_per_hour,omitempty"`
}
