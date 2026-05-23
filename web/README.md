# Quark Web

Quark Web is the browser client for the NATS-native Quark stack. It is a
Next.js app that connects directly to the local NATS WebSocket listener and
uses the same NATS subjects as the CLI and runtime contracts.

## Requirements

- Node.js 22.14 or newer
- npm 11.3 or newer
- A Quark supervisor or local NATS server with the WebSocket listener enabled

## Local Development

```bash
npm install
npm run dev
```

Open `http://localhost:3000/chat`.

The app expects these browser-safe environment variables:

| Variable                     | Purpose                                                       | Default               |
| ---------------------------- | ------------------------------------------------------------- | --------------------- |
| `NEXT_PUBLIC_NATS_WS_URL`    | NATS WebSocket URL used by `@nats-io/nats-core` `wsconnect()` | `ws://127.0.0.1:9222` |
| `NEXT_PUBLIC_NATS_USER`      | Local browser credential username                             | empty                 |
| `NEXT_PUBLIC_NATS_PASSWORD`  | Local browser credential password                             | empty                 |
| `NEXT_PUBLIC_QUARK_SPACE_ID` | Optional comma-separated space allowlist shown by the UI      | all spaces            |

Do not expose production control-plane credentials through `NEXT_PUBLIC_*`.
Browser credentials must be scoped by NATS permissions to the subjects the web
client actually needs.

## NATS Contract

All product data flow uses NATS. The browser opens a module-scope singleton
connection in `src/lib/nats/connection.ts`; React consumes that promise through
Suspense in `src/components/nats/nats-client-boundary.tsx`.

Main subjects used by the UI:

- `control.space.v1.list`
- `control.space.v1.credential`
- `control.session.v1.list`
- `control.session.v1.create`
- `control.session.v1.delete`
- `control.session.v1.credential`
- `runtime.info.v1.get`
- `runtime.plan.v1.get`
- `runtime.plan.v1.approve`
- `runtime.plan.v1.reject`
- `runtime.activity.v1.list`
- `session.<session_id>.input`
- `session.<session_id>.events`

The web project does not expose Next.js API proxy routes for Quark operations.
Requests, replies, and subscriptions are implemented under `src/lib/nats/`.

## Verification

```bash
npm run lint
npm run format:check
npx tsc --noEmit
npm run build
npm run e2e
npm audit --audit-level=moderate
```

The E2E suite starts a real local `nats-server`, connects the browser over
WebSocket, connects the harness through `@nats-io/transport-node`, and records
NATS CLI diagnostics for subject/subscription inspection.
