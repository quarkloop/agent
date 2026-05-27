# Running & Debugging

Practical techniques for running, testing, and debugging Quark locally.

## 1. Building

```bash
make build
```

Binaries land in `./bin/`:

| Binary | Role |
| --- | --- |
| `supervisor` | NATS-native supervisor control plane and embedded broker |
| `runtime` | agent runtime process |
| `quark` | CLI |
| `bash`, `fs`, `web-search` | tool plugin binaries |
| `indexer-service`, `gateway-service`, `space-service`, `runstate-service` | NATS service-function hosts |

Tool plugin `.so` files and provider `.so` files are built with:

```bash
make build-plugins
```

Regenerate protobuf payload bindings after proto changes:

```bash
make proto
```

## 2. Starting The Supervisor

The supervisor is the normal control-plane entrypoint for local development.
It owns its embedded NATS hub, account setup, catalogs, and JetStream
resources. Runtime and service processes are launched by deployment tooling.

```bash
./bin/supervisor start --bundled-plugins-dir ./plugins
```

The embedded NATS hub uses supervisor-owned state, not user workspace
directories:

```bash
./bin/supervisor start \
  --bundled-plugins-dir ./plugins \
  --installed-plugins-dir /tmp/quark-supervisor/plugins \
  --nats-state-dir /tmp/quark-supervisor/nats \
  --nats-client-port 4222 \
  --nats-websocket-port 9222 \
  --nats-monitor-port 8222
```

Use an external NATS server when testing deployment wiring:

```bash
./bin/supervisor start --nats-mode external --nats-url nats://127.0.0.1:4222
```

The CLI control path uses NATS:

```bash
export QUARK_NATS_URL=nats://127.0.0.1:4222
export QUARK_NATS_USER=quark-control
export QUARK_NATS_PASSWORD=quark-control-dev
```

The supervisor exposes no product HTTP control API and launches no runtime or
service process. Runtime and service process lifecycle is deployment-owned.
Use `deploy/compose/quark.yml` or the units under `deploy/systemd/`.

## 3. Starting Services

Services expose protobuf-backed NATS service functions described by service
plugins. All Go application transport is hosted through `pkg/natskit`; service
operations use `svc.<service>.v1.<function>`, and responder queue groups are
derived by `pkg/natskit` as `q.service.v1.<service>` and are not caller or
instance configuration.

```bash
# Requires Dgraph Alpha on 127.0.0.1:9080
./bin/indexer-service --dgraph 127.0.0.1:9080 --skill-dir plugins/services/indexer \
  --nats-url "$QUARK_NATS_URL" --nats-user "$QUARK_NATS_USER" --nats-password "$QUARK_NATS_PASSWORD"

export QUARK_GATEWAY_EMBEDDING_PROVIDER=openrouter
export OPENROUTER_EMBEDDING_MODEL=nvidia/llama-nemotron-embed-vl-1b-v2:free
./bin/gateway-service --skill-dir plugins/services/gateway \
  --nats-url "$QUARK_NATS_URL" --embedding-provider "$QUARK_GATEWAY_EMBEDDING_PROVIDER"

./bin/runstate-service --root /tmp/quark-runstate \
  --skill-dir plugins/services/runstate --nats-url "$QUARK_NATS_URL" \
  --nats-user "$QUARK_NATS_USER" --nats-password "$QUARK_NATS_PASSWORD"
```

See [services.md](services.md) for service topology, protobuf conventions,
lifecycle, and E2E instructions.

## 4. Tool Servers

Tool plugins can run in api mode as standalone HTTP servers. The runtime prefers
lib mode when a `.so` is installed next to the manifest and falls back to api
mode otherwise.

```bash
./bin/bash serve --addr 127.0.0.1:8091
./bin/fs serve --addr 127.0.0.1:8093
./bin/web-search serve --addr 127.0.0.1:8090
```

Set `BRAVE_API_KEY` or `SERPAPI_KEY` for real web search results; otherwise the
web-search plugin uses its stub behavior.

## 5. Starting A Runtime Directly

Runtime processes are started by deployment configuration after the supervisor
has provisioned NATS credentials and catalogs. Direct startup is useful for
debugging the agent process itself.

```bash
export QUARK_MODEL_PROVIDER=openrouter
export QUARK_MODEL_NAME=openai/gpt-4o-mini
export QUARK_SPACE=my-space
export QUARK_NATS_URL=nats://127.0.0.1:4222
export QUARK_NATS_USER=<runtime-user>
export QUARK_NATS_PASSWORD=<runtime-password>
export OPENROUTER_API_KEY=sk-or-v1-...

./bin/runtime start --channel nats
```

The NATS channel listens on `session.*.input` and emits
`session.<session-id>.events`. Runtime product startup accepts the NATS
channel only; the former HTTP client and web-channel packages have been
removed.

## 6. CLI Flow

```bash
export QUARK_NATS_URL=nats://127.0.0.1:4222
export QUARK_NATS_USER=quark-control
export QUARK_NATS_PASSWORD=quark-control-dev

mkdir /tmp/my-space
quark init my-space --work-dir /tmp/my-space
export QUARK_SPACE=my-space
quark chat "summarize this space"
quark activity query --follow
```

All `quark` commands operate on the space selected with `--space` or
`QUARK_SPACE`. The Space service persists the authoritative `space.json`
record; product paths do not write configuration into a user working
directory. Commands create spaces/sessions and stream chat over NATS
subjects. Chat
requests first issue a session-scoped credential through
`control.session.v1.credential`, then send the prompt to
`session.<session-id>.input` and subscribe to `session.<session-id>.events`.
Runtime inspection commands issue a space-scoped credential before using
`runtime.info.v1.get`, `runtime.session.v1.get`, `runtime.plan.v1.*`, and
`runtime.activity.v1.*`.

Runtime startup code is intentionally split by ownership. `runtime/pkg/commands`
parses Cobra input only; `runtime/pkg/startup` owns environment value capture,
channel validation, space credentials, runtime leases, catalog loading, profile
selection, service-function wiring, and agent/channel construction.

Within `runtime/pkg/agent`, `agent.go` owns construction and loop lifecycle
only. Request inference lives in `request.go`, tool policy/execution in
`tools.go`, activity/model accounting in `activity.go`, prompt composition in
`prompting.go`, initialization in `initialization.go`, delegation/work
scheduling in `delegation.go` and `work.go`. Sessions enter through NATS
message operations; runtime has no supervisor HTTP session-mirroring feed.

Within `runtime/pkg/workflow`, `state.go` owns transient workflow state and
copy-safe transitions, `tracker.go` correlates successful tool results and
events, `detection.go` recognizes required workflows from the resolved tool
surface, `policy.go` enforces ordered progression, `prompting.go` emits
structured workflow-status facts for Harness packaging, and `observation.go` parses
tool-result evidence. This runtime layer does not publish NATS operations or
own durable workflow history; `services/workflow` owns durable execution and
uses `pkg/natskit` service-function operations.

Supervisor NATS control handlers are also split by subject family. Core
connection, subscription, envelope, and response behavior stays in
`supervisor/pkg/natsapi/server.go`; spaces, sessions, plugins, services,
catalogs, and contract conversion each live in focused package files. Subject
registration remains explicit in one table.

CLI NATS client code follows the same split. `cli/pkg/natsclient/client.go`
owns connection setup, request envelopes, response errors, and shared helpers;
space, session, runtime, Harness context inspection, plugin, and service operations live in focused
files. CLI command packages should call these methods instead of handling NATS
envelopes directly.

The E2E harness keeps fixture concerns separate. `e2e/utils/env.go` owns the
full fixture lifecycle, `supervisor.go` starts the NATS control plane,
`space_config.go` generates Space-service-owned configuration, `process.go`
owns subprocess lifecycle/logging, `process_env.go` owns environment
filtering, `network.go` owns readiness probes, and `nats_cli.go` owns NATS CLI
diagnostics. Plugin assets are installed in the supervisor registry, then
selected by space configuration; no per-space plugin directory is authoritative.

The runtime NATS channel is also transport-separated. `pkg/natskit` owns
`ApplicationHost`, application route registration, event publication,
JetStream acknowledgement, readiness, and draining. In
`runtime/pkg/channel/nats`, `channel.go` binds client-contract routes,
`handlers.go` maps inbound envelopes to runtime behavior, `events.go` maps
session/activity events with correlation metadata, `mapping.go` maps runtime
domain state to client DTOs, and `config.go` owns channel configuration.
Session and runtime-activity histories are provisioned by supervisor within
each space NATS account; request/reply subjects are never included in those
history streams.

The embedded broker implementation is split inside `supervisor/pkg/natshub`
by responsibility: lifecycle, endpoints, account/authentication policy,
credential issuance, import/export visibility, and JetStream storage policy.
Session and runtime-activity histories are stored in each space account.
The `runtime_space_leases` KV bucket is also per-space so the scoped runtime
credential can coordinate competing runtimes for that space. The
`runstate_leases` KV bucket remains in the control account and is accessible
only to the Run State service storage path.

Browser and mobile clients use the same subjects over the embedded NATS
WebSocket listener:

```bash
export QUARK_NATS_WEBSOCKET_URL=ws://127.0.0.1:9222
```

There is no runtime HTTP product message API.

## 7. Testing Via NATS CLI

Health and request/reply checks should use the NATS CLI against the same
subjects used by the product clients:

```bash
nats --server "$QUARK_NATS_URL" \
  --user "$QUARK_NATS_USER" \
  --password "$QUARK_NATS_PASSWORD" \
  request runtime.info.v1.get \
  '{"version":"v1","request_id":"manual-runtime-info","actor":"debug","payload":{"space_id":"my-space"}}'
```

Session input is sent with a session-scoped credential:

```bash
nats --server "$QUARK_NATS_URL" \
  --user "$QUARK_NATS_USER" \
  --password "$QUARK_NATS_PASSWORD" \
  request session.chat.input \
  '{"version":"v1","request_id":"manual-chat","space_id":"my-space","session_id":"chat","actor":"debug","payload":{"content":"what time is it?"}}'
```

Use system credentials for server and subject investigation:

```bash
nats --server "$QUARK_NATS_URL" \
  --user "$QUARK_NATS_SYSTEM_USER" \
  --password "$QUARK_NATS_SYSTEM_PASSWORD" \
  server request subscriptions --filter-subject session.chat.input --detail 1
```

Service-function responses include `reference_id` and `audit_ref` values for
durable audit lookup. Embedded supervisor audit retention is configurable
without exposing payload bodies:

```bash
./bin/supervisor start --nats-audit-retention 2160h --nats-audit-max-messages 10000000
quark audit get <reference-id> --space my-space
quark audit list --space my-space --session <session-id> --limit 20
quark audit retention
```

Audit CLI output exposes service-call identity, correlation, status, timing,
and retention metadata only. Durable payload snapshots are not exposed by the
CLI audit contract.

## 8. Running The Web UI

```bash
cd web
npm install
npm run dev
```

The dev server runs on `http://localhost:3000/chat`. It connects directly to
the embedded NATS WebSocket listener through `NEXT_PUBLIC_NATS_WS_URL`; it does
not proxy Quark operations through Next.js API routes.

## 9. Tests

Run workspace unit tests:

```bash
make test
```

Run E2E tests:

```bash
go test -tags e2e -v -timeout 10m ./e2e
```

Run the service-backed indexer PDF E2E:

```bash
go test -tags e2e -v -run '^TestAgentIndexesUploadedPDFDataset$' ./e2e
go test -tags e2e -v -run '^TestAgentIndexesUploadedPDFDatasetOpenRouterEmbedding$' ./e2e
```

The PDF test requires Docker/Dgraph through the E2E helper, `pdftotext` in
`PATH`, and available provider quota. It sends PDF paths to the runtime agent,
verifies the agent uses document/IO extraction, Gateway embedding, indexing, and
retrieval service calls, queries the real Dgraph-backed indexer after the agent
run, and logs artifact paths for manual verification. The OpenRouter embedding
variant uses `OPENROUTER_E2E_EMBEDDING_MODEL`; no deterministic embedding
fallback is permitted. Provider 429 responses are treated as
unavailable external dependencies.

## 10. Debugging Tips

- E2E logs use explicit ownership prefixes:
  - `[e2e][test=<name>]` for test harness logs.
  - `[e2e][test=<name>][process=<name>]` for subprocess stdout/stderr.
  - `[parent_process=supervisor]` when a runtime child log is inherited through
    the supervisor process pipe.
  Runtime and supervisor structured logs include a `process` attribute so tool
  calls are visibly runtime-owned even when the supervisor launched the runtime.
- Use `pgrep -af 'bin/runtime|bin/supervisor|indexer-service|space-service'`
  to find running Quark processes.
- Port conflicts usually show up as `bind: address already in use`; stop the
  old process or choose another port.
- Supervisor-owned service discovery logs one line per discovered service. If a
  service is missing from the agent prompt or a generated service tool cannot
  resolve it, check the relevant `QUARK_*_ADDR` environment variable and that
  `ServiceRegistry.ListServices` is reachable before runtime starts.
- Provider errors such as 405, 429, or "Function calling is not enabled" are
  upstream model/provider issues. Try a model that supports tool calling or
  wait for rate limits to clear.
- If old session or context data causes confusion, inspect Harness context
  reports with `quark context reports <session-id>` and inspect session events.
