import {
  type ConnectionOptions,
  type NatsConnection,
  wsconnect,
} from "@nats-io/nats-core";
import { browserNatsConfig } from "@/lib/nats/config";
import type { NatsCredential } from "@/lib/nats/types";

let controlConnectionPromise: Promise<NatsConnection> | null = null;
const credentialConnectionPromises = new Map<string, Promise<NatsConnection>>();

export function controlConnection(): Promise<NatsConnection> {
  if (!controlConnectionPromise) {
    controlConnectionPromise = connectWithOptions("quark-web-control");
  }
  return controlConnectionPromise;
}

export function credentialConnection(
  credential: NatsCredential,
): Promise<NatsConnection> {
  const key = credentialKey(credential);
  let promise = credentialConnectionPromises.get(key);
  if (!promise) {
    promise = connectWithOptions(`quark-web-${credential.role}`, credential);
    credentialConnectionPromises.set(key, promise);
  }
  return promise;
}

async function connectWithOptions(
  name: string,
  credential?: Pick<NatsCredential, "username" | "password">,
): Promise<NatsConnection> {
  const cfg = browserNatsConfig();
  const user = credential?.username || cfg.username;
  const pass = credential?.password || cfg.password;
  const options: ConnectionOptions = {
    servers: [cfg.wsUrl],
    name,
    reconnect: true,
    reconnectDelayHandler: () => 250,
    maxReconnectAttempts: -1,
  };
  if (user) options.user = user;
  if (pass) options.pass = pass;
  return wsconnect(options);
}

function credentialKey(credential: NatsCredential): string {
  return [
    credential.account,
    credential.role,
    credential.space_id,
    credential.session_id,
    credential.username,
  ]
    .filter(Boolean)
    .join(":");
}
