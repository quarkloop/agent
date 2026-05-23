import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.WEB_E2E_PORT ?? 3100);

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  expect: {
    timeout: 10_000,
  },
  reporter: [["list"]],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: "on-first-retry",
  },
  webServer: {
    command: `npm run dev -- --hostname 127.0.0.1 --port ${port}`,
    url: `http://127.0.0.1:${port}/chat`,
    timeout: 60_000,
    reuseExistingServer: !process.env.CI,
    env: {
      NEXT_PUBLIC_NATS_WS_URL:
        process.env.NEXT_PUBLIC_NATS_WS_URL ?? "ws://127.0.0.1:9224",
      NEXT_PUBLIC_NATS_USER: process.env.NEXT_PUBLIC_NATS_USER ?? "app",
      NEXT_PUBLIC_NATS_PASSWORD: process.env.NEXT_PUBLIC_NATS_PASSWORD ?? "app",
    },
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
