import type { NatsConnection } from "@nats-io/nats-core";
import { browserNatsConfig } from "@/lib/nats/config";
import { controlConnection, credentialConnection } from "@/lib/nats/connection";
import { requestPayload } from "@/lib/nats/envelope";
import {
  sessionEventsSubject,
  sessionInputSubject,
  SUBJECTS,
} from "@/lib/nats/subjects";
import type {
  CredentialResponse,
  CreateSessionRequest,
  ListSessionsResponse,
  ListSpacesResponse,
  NatsCredential,
  RuntimeActivityListResponse,
  RuntimeInfoResponse,
  RuntimePlanResponse,
  SendMessageRequest,
  SendMessageResponse,
  SessionEvent,
  SessionInfo,
  SpaceInfo,
  WebAgentConnection,
} from "@/lib/nats/types";
import type {
  CreateSessionResponse,
  SessionRecord,
  SessionType,
} from "@/lib/types";

const credentialCache = new Map<string, Promise<NatsCredential>>();

export async function listAgents(): Promise<WebAgentConnection[]> {
  const spaces = await listSpaces();
  const configured = configuredSpaceFilter();
  const filtered = configured
    ? spaces.filter((space) => configured.has(space.name))
    : spaces;

  return Promise.all(filtered.map(spaceToAgent));
}

export async function listSpaces(): Promise<SpaceInfo[]> {
  const conn = await controlConnection();
  const resp = await requestPayload<ListSpacesResponse>(
    conn,
    SUBJECTS.spaceList,
    undefined,
    {},
  );
  return [...resp.spaces];
}

export async function listSessions(spaceID: string): Promise<SessionRecord[]> {
  const conn = await controlConnection();
  const resp = await requestPayload<ListSessionsResponse>(
    conn,
    SUBJECTS.sessionList,
    spaceID,
    { space_id: spaceID },
  );
  return resp.sessions.map((session) => normalizeSession(spaceID, session));
}

export async function createSession(
  spaceID: string,
  req: { type: SessionType; title?: string },
): Promise<CreateSessionResponse> {
  const conn = await controlConnection();
  const payload: CreateSessionRequest = {
    space_id: spaceID,
    type: req.type,
    title: req.title,
  };
  const session = await requestPayload<SessionInfo>(
    conn,
    SUBJECTS.sessionCreate,
    spaceID,
    payload,
  );
  return { session: normalizeSession(spaceID, session) };
}

export async function deleteSession(
  spaceID: string,
  sessionID: string,
): Promise<void> {
  const conn = await controlConnection();
  await requestPayload<Record<string, never>>(
    conn,
    SUBJECTS.sessionDelete,
    spaceID,
    { space_id: spaceID, session_id: sessionID },
  );
}

export async function runtimeInfo(
  spaceID: string,
): Promise<RuntimeInfoResponse> {
  return requestWithSpaceCredential<RuntimeInfoResponse>(
    spaceID,
    SUBJECTS.runtimeInfoGet,
    { space_id: spaceID },
  );
}

export async function runtimePlan(
  spaceID: string,
): Promise<RuntimePlanResponse> {
  return requestWithSpaceCredential<RuntimePlanResponse>(
    spaceID,
    SUBJECTS.runtimePlanGet,
    { space_id: spaceID },
  );
}

export async function approveRuntimePlan(
  spaceID: string,
): Promise<RuntimePlanResponse> {
  return requestWithSpaceCredential<RuntimePlanResponse>(
    spaceID,
    SUBJECTS.runtimePlanApprove,
    { space_id: spaceID },
  );
}

export async function rejectRuntimePlan(
  spaceID: string,
): Promise<RuntimePlanResponse> {
  return requestWithSpaceCredential<RuntimePlanResponse>(
    spaceID,
    SUBJECTS.runtimePlanReject,
    { space_id: spaceID },
  );
}

export async function runtimeActivity(
  spaceID: string,
  limit: number,
): Promise<RuntimeActivityListResponse> {
  return requestWithSpaceCredential<RuntimeActivityListResponse>(
    spaceID,
    SUBJECTS.runtimeActivityList,
    { space_id: spaceID, limit },
  );
}

export async function sendSessionMessage(req: SendMessageRequest) {
  const conn = await sessionConnection(req.space_id, req.session_id);
  return requestPayload<SendMessageResponse>(
    conn,
    sessionInputSubject(req.session_id),
    req.space_id,
    req,
    { sessionID: req.session_id },
  );
}

export async function subscribeSessionEvents(
  spaceID: string,
  sessionID: string,
  onEvent: (event: SessionEvent) => void,
  onError: (error: Error) => void,
): Promise<() => void> {
  const conn = await sessionConnection(spaceID, sessionID);
  const subscription = conn.subscribe(sessionEventsSubject(sessionID), {
    callback: (err, msg) => {
      if (err) {
        onError(err);
        return;
      }
      try {
        onEvent(msg.json<SessionEvent>());
      } catch (error) {
        onError(error instanceof Error ? error : new Error(String(error)));
      }
    },
  });
  await conn.flush();
  return () => subscription.unsubscribe();
}

export async function requestWithSpaceCredential<T>(
  spaceID: string,
  subject: string,
  payload: unknown,
): Promise<T> {
  const conn = await spaceConnection(spaceID);
  return requestPayload<T>(conn, subject, spaceID, payload);
}

export async function spaceConnection(
  spaceID: string,
): Promise<NatsConnection> {
  return credentialConnection(await spaceCredential(spaceID));
}

export async function sessionConnection(
  spaceID: string,
  sessionID: string,
): Promise<NatsConnection> {
  return credentialConnection(await sessionCredential(spaceID, sessionID));
}

async function spaceToAgent(space: SpaceInfo): Promise<WebAgentConnection> {
  const port = natsPort();
  let status: WebAgentConnection["status"] = "unknown";
  try {
    await runtimeInfo(space.name);
    status = "online";
  } catch {
    status = "offline";
  }
  return {
    id: space.name,
    name: space.name,
    mode: "direct",
    baseUrl: browserNatsConfig().wsUrl,
    natsUrl: browserNatsConfig().wsUrl,
    port,
    status,
    spaceId: space.name,
  };
}

function normalizeSession(
  spaceID: string,
  session: SessionInfo,
): SessionRecord {
  return {
    id: session.id,
    key: session.id,
    agent_id: spaceID,
    space: spaceID,
    type: session.type,
    status: "open",
    title: session.title,
    created_at: session.created_at,
    updated_at: session.updated_at,
  };
}

async function spaceCredential(spaceID: string): Promise<NatsCredential> {
  return cachedCredential(`space:${spaceID}`, async () => {
    const conn = await controlConnection();
    const resp = await requestPayload<CredentialResponse>(
      conn,
      SUBJECTS.spaceCredential,
      spaceID,
      { space_id: spaceID },
    );
    return resp.credential;
  });
}

async function sessionCredential(
  spaceID: string,
  sessionID: string,
): Promise<NatsCredential> {
  return cachedCredential(`session:${spaceID}:${sessionID}`, async () => {
    const conn = await controlConnection();
    const resp = await requestPayload<CredentialResponse>(
      conn,
      SUBJECTS.sessionCredential,
      spaceID,
      { space_id: spaceID, session_id: sessionID },
      { sessionID },
    );
    return resp.credential;
  });
}

function cachedCredential(
  key: string,
  load: () => Promise<NatsCredential>,
): Promise<NatsCredential> {
  let promise = credentialCache.get(key);
  if (!promise) {
    promise = load();
    credentialCache.set(key, promise);
  }
  return promise;
}

function configuredSpaceFilter(): Set<string> | null {
  const configured = process.env.NEXT_PUBLIC_QUARK_SPACE_ID?.trim();
  if (!configured) return null;
  return new Set(
    configured
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean),
  );
}

function natsPort(): number {
  const port = new URL(browserNatsConfig().wsUrl).port;
  return port ? Number(port) : 0;
}
