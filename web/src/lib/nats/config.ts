export const DEFAULT_NATS_WS_URL = "ws://127.0.0.1:9222";
export const DEFAULT_REQUEST_TIMEOUT_MS = 5_000;

export interface BrowserNatsConfig {
  wsUrl: string;
  username?: string;
  password?: string;
}

export function browserNatsConfig(): BrowserNatsConfig {
  return {
    wsUrl: normalizeWebSocketURL(
      process.env.NEXT_PUBLIC_NATS_WS_URL ?? DEFAULT_NATS_WS_URL,
    ),
    username: nonEmpty(process.env.NEXT_PUBLIC_NATS_USER),
    password: nonEmpty(process.env.NEXT_PUBLIC_NATS_PASSWORD),
  };
}

export function defaultSpaceID(): string | undefined {
  return nonEmpty(process.env.NEXT_PUBLIC_QUARK_SPACE_ID);
}

export function normalizeWebSocketURL(raw: string): string {
  const value = raw.trim();
  if (!value) return DEFAULT_NATS_WS_URL;
  const parsed = new URL(value);
  if (parsed.protocol !== "ws:" && parsed.protocol !== "wss:") {
    throw new Error("NEXT_PUBLIC_NATS_WS_URL must use ws:// or wss://");
  }
  parsed.pathname = parsed.pathname.replace(/\/+$/, "");
  parsed.search = "";
  parsed.hash = "";
  return parsed.toString().replace(/\/$/, "");
}

function nonEmpty(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}
