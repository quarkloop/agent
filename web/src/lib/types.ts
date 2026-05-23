// ─── Connection ───────────────────────────────────────────────

export type ConnectionMode = "direct" | "proxied";
export type AgentStatus = "online" | "offline" | "busy" | "unknown";

export interface AgentConnection {
  id: string;
  name: string;
  mode: ConnectionMode;
  baseUrl: string;
  port: number;
  status: AgentStatus;
  spaceId?: string;
  runtimeId?: string;
  provider?: string;
  model?: string;
}

// ─── Chat ─────────────────────────────────────────────────────

export type AgentMode = "ask" | "plan" | "masterplan" | "auto";

export interface ChatRequest {
  message: string;
  session_key?: string;
  stream?: boolean;
  mode?: AgentMode;
}

export interface ChatResponse {
  reply: string;
  mode?: string;
  warning?: string;
  input_tokens?: number;
  output_tokens?: number;
}

export interface HealthResponse {
  agent_id?: string;
  status: string;
}

export interface ModeResponse {
  mode: string;
}

export type StatsResponse = Record<string, unknown>;

// ─── Plan ─────────────────────────────────────────────────────

export type PlanStatus =
  | "draft"
  | "approved"
  | "rejected"
  | "executing"
  | "succeeded"
  | "failed";
export type StepStatus = "pending" | "running" | "complete" | "failed";

export interface Step {
  id: string;
  agent: string;
  description: string;
  depends_on: string[];
  status: StepStatus;
  result?: string;
  error?: string;
  started_at?: string;
  finished_at?: string;
}

export interface Plan {
  goal: string;
  status: PlanStatus;
  steps: Step[];
  complete: boolean;
  summary?: string;
  created_at: string;
  updated_at: string;
}

// ─── Activity ─────────────────────────────────────────────────

export const ACTIVITY_TYPES = {
  SESSION_STARTED: "session.started",
  SESSION_ENDED: "session.ended",
  MESSAGE_ADDED: "message.added",
  TOOL_CALLED: "tool.called",
  TOOL_COMPLETED: "tool.completed",
  PLAN_CREATED: "plan.created",
  PLAN_UPDATED: "plan.updated",
  STEP_DISPATCHED: "step.dispatched",
  STEP_COMPLETED: "step.completed",
  STEP_FAILED: "step.failed",
  CONTEXT_COMPACTED: "context.compacted",
  CHECKPOINT_SAVED: "checkpoint.saved",
  MODE_CLASSIFIED: "mode.classified",
  MASTERPLAN_CREATED: "masterplan.created",
  PHASE_STARTED: "phase.started",
  PHASE_COMPLETED: "phase.completed",
  PHASE_FAILED: "phase.failed",
} as const;

export type ActivityType = (typeof ACTIVITY_TYPES)[keyof typeof ACTIVITY_TYPES];

export interface ActivityRecord {
  id: string;
  session_id?: string;
  type: ActivityType | string;
  timestamp: string;
  data?: Record<string, string>;
}

// ─── Session ─────────────────────────────────────────────────

export type SessionType = "main" | "chat" | "task" | "subagent" | "cron";

export interface SessionRecord {
  id: string;
  key: string;
  agent_id: string;
  space?: string;
  type: SessionType;
  status: string;
  title?: string;
  created_at: string;
  updated_at: string;
  ended_at?: string;
}

export interface CreateSessionRequest {
  type: SessionType;
  title?: string;
}

export interface CreateSessionResponse {
  session?: SessionRecord;
}

// ─── Space (api-server) ───────────────────────────────────────

export type SpaceStatus =
  | "created"
  | "starting"
  | "running"
  | "stopping"
  | "stopped"
  | "failed";

export interface Space {
  id: string;
  name: string;
  dir: string;
  status: SpaceStatus;
  pid?: number;
  port?: number;
  created_at: string;
  updated_at: string;
  restart_policy?: string;
}

// ─── Error ────────────────────────────────────────────────────

export interface ErrorResponse {
  error: string;
}

// ─── File attachment ──────────────────────────────────────────

export interface FileAttachment {
  name: string;
  mimeType: string;
  size: number;
  file: File;
}
