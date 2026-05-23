import type { NatsConnection } from "@nats-io/nats-core";
import { DEFAULT_REQUEST_TIMEOUT_MS } from "@/lib/nats/config";
import type { RequestEnvelope, ResponseEnvelope } from "@/lib/nats/types";

export class NatsResponseError extends Error {
  constructor(
    readonly category: string,
    message: string,
  ) {
    super(category ? `${category}: ${message}` : message);
    this.name = "NatsResponseError";
  }
}

export async function requestPayload<T>(
  connection: NatsConnection,
  subject: string,
  spaceID: string | undefined,
  payload: unknown,
  options: { sessionID?: string; timeout?: number } = {},
): Promise<T> {
  const envelope = newRequestEnvelope(spaceID, payload, options.sessionID);
  const msg = await connection.request(subject, JSON.stringify(envelope), {
    timeout: options.timeout ?? DEFAULT_REQUEST_TIMEOUT_MS,
  });
  const response = msg.json<ResponseEnvelope<T>>();
  validateResponse(response, envelope.request_id);
  if (response.status === "error") {
    throw new NatsResponseError(
      response.error?.category ?? "unknown",
      response.error?.message ?? "NATS request failed",
    );
  }
  return response.payload as T;
}

function newRequestEnvelope(
  spaceID: string | undefined,
  payload: unknown,
  sessionID?: string,
): RequestEnvelope {
  return {
    version: "v1",
    request_id: newRequestID(),
    space_id: spaceID,
    session_id: sessionID,
    actor: "web",
    payload,
  };
}

function validateResponse(response: ResponseEnvelope, requestID: string) {
  if (response.version !== "v1") {
    throw new Error(`unsupported response version ${response.version}`);
  }
  if (response.request_id !== requestID) {
    throw new Error("response request_id does not match request");
  }
  if (response.status !== "ok" && response.status !== "error") {
    throw new Error(`invalid response status ${response.status}`);
  }
}

function newRequestID(): string {
  if (globalThis.crypto?.randomUUID) {
    return `req-${globalThis.crypto.randomUUID()}`;
  }
  const bytes = new Uint8Array(12);
  globalThis.crypto?.getRandomValues(bytes);
  const suffix = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join(
    "",
  );
  return `req-${suffix || Date.now().toString(36)}`;
}
