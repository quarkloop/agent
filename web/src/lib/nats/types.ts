import type {
  ActivityRecord,
  AgentConnection,
  Plan,
  SessionType,
} from "@/lib/types";

export type EnvelopeStatus = "ok" | "error";

export interface RequestEnvelope {
  version: "v1";
  request_id: string;
  space_id?: string;
  session_id?: string;
  actor?: string;
  traceparent?: string;
  tracestate?: string;
  payload: unknown;
}

export interface ResponseEnvelope<T = unknown> {
  version: "v1";
  request_id: string;
  status: EnvelopeStatus;
  payload?: T;
  error?: {
    category: string;
    message: string;
  };
}

export interface SpaceInfo {
  name: string;
  version?: string;
  working_dir?: string;
  created_at: string;
  updated_at: string;
}

export interface ListSpacesResponse {
  spaces: SpaceInfo[];
}

export interface NatsCredential {
  url?: string;
  username: string;
  password: string;
  account: string;
  role: string;
  space_id?: string;
  session_id?: string;
  agent_id?: string;
}

export interface CredentialResponse {
  credential: NatsCredential;
}

export interface SessionInfo {
  id: string;
  type: SessionType;
  title?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateSessionRequest {
  space_id: string;
  type: SessionType;
  title?: string;
}

export interface ListSessionsResponse {
  sessions: SessionInfo[];
}

export interface RuntimeInfoResponse {
  sessions: number;
}

export interface SendMessageRequest {
  space_id: string;
  session_id: string;
  content: string;
}

export interface SendMessageResponse {
  session_id: string;
  accepted: boolean;
}

export interface SessionEvent {
  type: string;
  session_id: string;
  run_id?: string;
  payload?: unknown;
}

export interface RuntimeActivityListResponse {
  records: RuntimeActivityRecord[];
}

export interface RuntimeActivityRecord {
  id: string;
  session_id?: string;
  type: string;
  timestamp: string;
  data?: unknown;
}

export type RuntimePlanResponse = Plan;

export interface WebAgentConnection extends AgentConnection {
  natsUrl: string;
}

export type ActivitySink = (record: ActivityRecord) => void;
