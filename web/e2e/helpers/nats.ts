import { spawn, type ChildProcess } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import type { TestInfo } from "@playwright/test";
import {
  connect,
  type Msg,
  type NatsConnection,
} from "@nats-io/transport-node";

export const e2eNats = {
  tcpUrl: "nats://127.0.0.1:4224",
  wsUrl: "ws://127.0.0.1:9224",
  monitorPort: 8224,
  systemUser: "sys",
  systemPassword: "sys",
  appUser: "app",
  appPassword: "app",
  restrictedUser: "restricted",
  restrictedPassword: "restricted",
} as const;

type RequestEnvelope = {
  version: "v1";
  request_id: string;
  space_id?: string;
  session_id?: string;
  actor?: string;
  payload?: unknown;
};

type ResponseEnvelope = {
  version: "v1";
  request_id: string;
  status: "ok" | "error";
  payload?: unknown;
  error?: {
    category: string;
    message: string;
  };
};

export type NatsE2EServer = {
  tcpUrl: string;
  wsUrl: string;
  stop: () => Promise<void>;
};

export async function startNatsServer(
  testInfo: TestInfo,
): Promise<NatsE2EServer> {
  const workDir = await mkdtemp(path.join(tmpdir(), "quark-web-nats-"));
  const configPath = path.join(workDir, "nats.conf");
  await writeFile(configPath, natsConfig(workDir));

  const bin = process.env.NATS_SERVER_BIN ?? "nats-server";
  const child = spawn(bin, ["-c", configPath], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  pipeLogs(testInfo, child, "nats-server");

  try {
    await waitForNats(child, e2eNats.tcpUrl);
  } catch (error) {
    child.kill("SIGTERM");
    await waitForExit(child);
    await rm(workDir, { recursive: true, force: true });
    throw error;
  }

  return {
    tcpUrl: e2eNats.tcpUrl,
    wsUrl: e2eNats.wsUrl,
    stop: async () => {
      child.kill("SIGTERM");
      await waitForExit(child);
      await rm(workDir, { recursive: true, force: true });
    },
  };
}

export async function connectHarness(): Promise<NatsConnection> {
  return connect({
    servers: e2eNats.tcpUrl,
    user: e2eNats.appUser,
    pass: e2eNats.appPassword,
    name: "quark-web-e2e-harness",
  });
}

export function replyJSON<TPayload>(
  msg: Msg,
  req: RequestEnvelope,
  payload: TPayload,
) {
  msg.respond(JSON.stringify(ok(req, payload)));
}

export function replyError(msg: Msg, req: RequestEnvelope, message: string) {
  msg.respond(
    JSON.stringify({
      version: "v1",
      request_id: req.request_id,
      status: "error",
      error: {
        category: "permission",
        message,
      },
    } satisfies ResponseEnvelope),
  );
}

export function parseRequest(msg: Msg): RequestEnvelope {
  return msg.json<RequestEnvelope>();
}

export function ok<TPayload>(
  req: RequestEnvelope,
  payload: TPayload,
): ResponseEnvelope {
  return {
    version: "v1",
    request_id: req.request_id,
    status: "ok",
    payload,
  };
}

export function subscribeResponder(
  conn: NatsConnection,
  subject: string,
  handler: (msg: Msg, req: RequestEnvelope) => void,
): () => void {
  const sub = conn.subscribe(subject, {
    callback: (err, msg) => {
      if (err) throw err;
      handler(msg, parseRequest(msg));
    },
  });
  return () => sub.unsubscribe();
}

export async function inspectNatsWithCLI(
  testInfo: TestInfo,
  label: string,
  subjects: string[] = [],
) {
  await runCLI(testInfo, label, ["rtt"]);
  await runCLI(testInfo, label, ["server", "ping"]);
  for (const subject of subjects) {
    await runCLI(testInfo, label, [
      "server",
      "request",
      "subscriptions",
      "--filter-subject",
      subject,
      "--detail",
      "1",
    ]);
  }
}

async function runCLI(testInfo: TestInfo, label: string, args: string[]) {
  const bin = process.env.NATS_CLI_BIN ?? "nats";
  const fullArgs = [
    "--server",
    e2eNats.tcpUrl,
    "--user",
    e2eNats.systemUser,
    "--password",
    e2eNats.systemPassword,
    "--timeout",
    "3s",
    ...args,
  ];
  const result = await runCommand(bin, fullArgs);
  testInfo.attach(`[e2e:web] nats-cli ${label} ${args.join(" ")}`, {
    body: result,
    contentType: "text/plain",
  });
}

async function waitForNats(child: ChildProcess, server: string) {
  const deadline = Date.now() + 10_000;
  let lastError: unknown;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`nats-server exited with code ${child.exitCode}`);
    }
    try {
      const conn = await connect({
        servers: server,
        user: e2eNats.appUser,
        pass: e2eNats.appPassword,
        timeout: 500,
      });
      await conn.close();
      return;
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  }
  throw new Error(`nats-server did not become ready: ${String(lastError)}`);
}

function waitForExit(child: ChildProcess): Promise<void> {
  if (child.exitCode !== null) return Promise.resolve();
  return new Promise((resolve) => {
    child.once("exit", () => resolve());
  });
}

function pipeLogs(testInfo: TestInfo, child: ChildProcess, name: string) {
  const write = (chunk: Buffer) => {
    const text = chunk
      .toString("utf8")
      .split(/\r?\n/)
      .filter(Boolean)
      .map((line) => `[e2e:web][${name}] ${line}`)
      .join("\n");
    if (!text) return;
    testInfo.attach(`[e2e:web] ${name}`, {
      body: `${text}\n`,
      contentType: "text/plain",
    });
  };
  child.stdout?.on("data", write);
  child.stderr?.on("data", write);
}

function runCommand(command: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString("utf8");
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString("utf8");
    });
    child.once("error", reject);
    child.once("exit", (code) => {
      const output = [stdout, stderr].filter(Boolean).join("\n");
      if (code === 0) {
        resolve(output);
        return;
      }
      reject(
        new Error(
          `${command} ${args.join(" ")} exited with ${code}\n${output}`,
        ),
      );
    });
  });
}

function natsConfig(workDir: string): string {
  const storeDir = path.join(workDir, "jetstream").replaceAll("\\", "\\\\");
  return `
port: 4224
server_name: WEB_E2E
http_port: ${e2eNats.monitorPort}
system_account: SYS
jetstream {
  store_dir: "${storeDir}"
}
accounts: {
  SYS: {
    users: [
      { user: "${e2eNats.systemUser}", password: "${e2eNats.systemPassword}" }
    ]
  }
  APP: {
    users: [
      { user: "${e2eNats.appUser}", password: "${e2eNats.appPassword}", permissions: { publish: ">", subscribe: ">" } }
      { user: "${e2eNats.restrictedUser}", password: "${e2eNats.restrictedPassword}", permissions: { publish: ["session.e2e_denied.input"], subscribe: [] } }
    ]
  }
}
websocket {
  port: 9224
  no_tls: true
  same_origin: false
}
`;
}
