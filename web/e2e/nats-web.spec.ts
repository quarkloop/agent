import { expect, test, type Page } from "@playwright/test";
import type { NatsConnection } from "@nats-io/transport-node";
import {
  connectHarness,
  e2eNats,
  inspectNatsWithCLI,
  replyJSON,
  startNatsServer,
  subscribeResponder,
  type NatsE2EServer,
} from "./helpers/nats";

const spaceID = "e2e_space";
const sessionID = "e2e_chat";
const deniedSessionID = "e2e_denied";

test.describe("NATS-native web client", () => {
  let server: NatsE2EServer;
  let conn: NatsConnection;
  let cleanup: Array<() => void> = [];
  let credentialMode: "normal" | "denied";

  test.beforeEach(async ({}, testInfo) => {
    credentialMode = "normal";
    server = await startNatsServer(testInfo);
    conn = await connectHarness();
    cleanup = registerQuarkResponders(conn, () => credentialMode);
  });

  test.afterEach(async () => {
    for (const stop of cleanup.splice(0).reverse()) stop();
    await conn?.close();
    await server?.stop();
  });

  test("shows the NATS connection lifecycle and discovers spaces", async ({
    page,
  }, testInfo) => {
    await inspectNatsWithCLI(testInfo, "startup");

    await page.goto("/chat");

    await expect(page.getByText("connected")).toBeVisible();
    await expect(
      page.getByRole("button", { name: /e2e_space/i }),
    ).toBeVisible();
  });

  test("sends user prompts over NATS and renders session events", async ({
    page,
  }, testInfo) => {
    const inputRequest = new Promise<string>((resolve) => {
      cleanup.push(
        subscribeResponder(conn, `session.${sessionID}.input`, (msg, req) => {
          const payload = req.payload as { content?: string };
          resolve(payload.content ?? "");
          replyJSON(msg, req, {
            session_id: sessionID,
            accepted: true,
          });
        }),
      );
    });

    await page.goto("/chat");
    await selectAgentAndWaitForSession(page);
    await inspectNatsWithCLI(testInfo, "chat-session", [
      `session.${sessionID}.events`,
      `session.${sessionID}.input`,
    ]);

    await page.getByRole("textbox").fill("Summarize the latest indexed notes");
    await page.keyboard.press("Enter");

    await expect
      .poll(() => inputRequest)
      .toBe("Summarize the latest indexed notes");

    conn.publish(
      `session.${sessionID}.events`,
      JSON.stringify({
        type: "text",
        session_id: sessionID,
        payload: "The indexed notes are available through NATS.",
      }),
    );
    await conn.flush();

    await expect(
      page.getByText("The indexed notes are available through NATS."),
    ).toBeVisible();
  });

  test("does not use legacy REST APIs or non-NATS websocket transports", async ({
    page,
  }) => {
    const forbidden: string[] = [];
    page.on("request", (request) => {
      const url = request.url();
      if (url.includes("/api/v1/")) forbidden.push(url);
      if (request.resourceType() === "websocket" && url !== e2eNats.wsUrl) {
        forbidden.push(url);
      }
    });

    await page.goto("/chat");
    await selectAgentAndWaitForSession(page);

    expect(forbidden).toEqual([]);
  });

  test("surfaces NATS credential failures instead of hanging", async ({
    page,
  }) => {
    credentialMode = "denied";
    await page.goto("/chat");
    await page.getByRole("button", { name: /e2e_space/i }).click();

    await expect(page.getByText("Something went wrong")).toBeVisible();
  });
});

function registerQuarkResponders(
  conn: NatsConnection,
  credentialMode: () => "normal" | "denied",
): Array<() => void> {
  const now = new Date().toISOString();
  const sessions: Array<{
    id: string;
    type: "chat";
    title: string;
    created_at: string;
    updated_at: string;
  }> = [];

  return [
    subscribeResponder(conn, "control.space.v1.list", (msg, req) => {
      replyJSON(msg, req, {
        spaces: [
          {
            name: spaceID,
            version: "v1",
            working_dir: "/tmp/quark-web-e2e",
            created_at: now,
            updated_at: now,
          },
        ],
      });
    }),
    subscribeResponder(conn, "control.space.v1.credential", (msg, req) => {
      replyJSON(msg, req, {
        credential: {
          username: e2eNats.appUser,
          password: e2eNats.appPassword,
          account: "APP",
          role: "space",
          space_id: spaceID,
        },
      });
    }),
    subscribeResponder(conn, "control.session.v1.list", (msg, req) => {
      if (credentialMode() === "denied") {
        replyJSON(msg, req, {
          sessions: [
            {
              id: deniedSessionID,
              type: "chat",
              title: "Denied",
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
          ],
        });
        return;
      }
      replyJSON(msg, req, { sessions });
    }),
    subscribeResponder(conn, "control.session.v1.create", (msg, req) => {
      const session = {
        id: sessionID,
        type: "chat" as const,
        title: "Chat",
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      sessions.push(session);
      replyJSON(msg, req, session);
    }),
    subscribeResponder(conn, "control.session.v1.credential", (msg, req) => {
      if (credentialMode() === "denied") {
        replyJSON(msg, req, {
          credential: {
            username: "missing-user",
            password: "bad-password",
            account: "APP",
            role: "session",
            space_id: spaceID,
            session_id: deniedSessionID,
          },
        });
        return;
      }
      replyJSON(msg, req, {
        credential: {
          username: e2eNats.appUser,
          password: e2eNats.appPassword,
          account: "APP",
          role: "session",
          space_id: spaceID,
          session_id: sessionID,
        },
      });
    }),
    subscribeResponder(conn, "runtime.info.v1.get", (msg, req) => {
      replyJSON(msg, req, { sessions: sessions.length });
    }),
    subscribeResponder(conn, "runtime.activity.v1.list", (msg, req) => {
      replyJSON(msg, req, { records: [] });
    }),
    subscribeResponder(conn, "runtime.plan.v1.get", (msg, req) => {
      replyJSON(msg, req, {
        goal: "",
        status: "draft",
        steps: [],
        complete: false,
        created_at: now,
        updated_at: now,
      });
    }),
  ];
}

async function selectAgentAndWaitForSession(page: Page) {
  await page.getByRole("button", { name: /e2e_space/i }).click();
  await expect(page.getByText("Send a message to get started.")).toBeVisible();
}
