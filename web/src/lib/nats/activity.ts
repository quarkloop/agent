import type { ActivityRecord } from "@/lib/types";
import type { RuntimeActivityRecord, SessionEvent } from "@/lib/nats/types";

type ActivityData = Record<string, string>;

export function runtimeActivityToUI(
  record: RuntimeActivityRecord,
): ActivityRecord {
  return {
    id: record.id,
    session_id: record.session_id,
    type: normalizeActivityType(record.type),
    timestamp: normalizeTimestamp(record.timestamp),
    data: objectToStringRecord(record.data),
  };
}

export function userMessageActivity(
  sessionID: string,
  content: string,
): ActivityRecord {
  return {
    id: `${sessionID}:user:${Date.now()}`,
    session_id: sessionID,
    type: "message.added",
    timestamp: new Date().toISOString(),
    data: {
      author: "user",
      content,
    },
  };
}

export function assistantMessageActivity(
  sessionID: string,
  content: string,
  id: string,
): ActivityRecord {
  return {
    id,
    session_id: sessionID,
    type: "message.added",
    timestamp: new Date().toISOString(),
    data: {
      author: "agent",
      content,
    },
  };
}

export function sessionEventToActivity(
  event: SessionEvent,
): ActivityRecord | null {
  if (event.type === "tool_start") {
    const data = objectToStringRecord(event.payload);
    return {
      id: `${event.session_id}:tool:${data.id || data.name || Date.now()}:start`,
      session_id: event.session_id,
      type: "tool.called",
      timestamp: data.observed_at || new Date().toISOString(),
      data: {
        step: data.id || "tool",
        tool: data.name || "unknown",
        args: data.arguments || "",
      },
    };
  }

  if (event.type === "tool_result") {
    const data = objectToStringRecord(event.payload);
    return {
      id: `${event.session_id}:tool:${data.id || data.name || Date.now()}:result`,
      session_id: event.session_id,
      type: "tool.completed",
      timestamp: data.observed_at || new Date().toISOString(),
      data: {
        step: data.id || "tool",
        tool: data.name || "unknown",
        result: data.result || "",
        is_error: String(data.error === "true" || data.error === "1"),
      },
    };
  }

  if (event.type === "error") {
    const data = objectToStringRecord(event.payload);
    return assistantMessageActivity(
      event.session_id,
      data.message || data.error || "Agent returned an error.",
      `${event.session_id}:error:${Date.now()}`,
    );
  }

  return null;
}

export function eventText(event: SessionEvent): string {
  if (event.type !== "token" && event.type !== "text") return "";
  if (typeof event.payload === "string") return event.payload;
  return "";
}

function normalizeActivityType(kind: string): string {
  switch (kind) {
    case "tool_start":
      return "tool.called";
    case "tool_result":
      return "tool.completed";
    default:
      return kind;
  }
}

function normalizeTimestamp(value: string): string {
  if (!value) return new Date().toISOString();
  return value;
}

function objectToStringRecord(value: unknown): ActivityData {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const out: ActivityData = {};
  for (const [key, raw] of Object.entries(value)) {
    if (raw == null) continue;
    out[key] = typeof raw === "string" ? raw : JSON.stringify(raw);
  }
  return out;
}
