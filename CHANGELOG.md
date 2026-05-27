# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-04-01

### Added

- **12-module Go workspace** — `core`, `agent`, `agent-api`, `agent-client`, `api-server`, `cli`, `tools/bash`, `tools/kb`, `tools/read`, `tools/space`, `tools/write`, `tools/web-search`
- **Continuous planning cycle** — `ORIENT → PLAN → DISPATCH → MONITOR → ASSESS` supervisor loop
- **Four working modes** — `ask`, `plan`, `masterplan`, `auto` (per-session, LLM-classified in auto mode)
- **Historical session model** - `main`, `chat`, `subagent`, `cron` types with hierarchical context keys (superseded by Harness memory)
- **LLM context management** — freshness policies (TTL, linear/exponential/step/position decay), token-aware compaction, multiple token estimators
- **Tool binaries** — `bash`, `read`, `write`, `web-search`, `kb`, `space` (CLI + HTTP server modes)
- **LLM providers** - Anthropic, OpenAI, OpenRouter
- **Historical space DSL** - declarative configuration design (superseded by Space-service-owned `space.json`)
- **Provider-free test adapter** - historical pre-Gateway test mechanism, removed from the current product
- **SSE activity streams** — real-time structured agent activity per session
- **Ring-buffer log tailing** — `quark logs <id>` streams live process output
- **Restart policies** — `on-failure` (default), `always`, `never` — max 5 restarts with 10 s cooldown
- **Approval policies** — `required` (draft plans await user approval) and `auto`
- **Historical context inspection CLI** - removed; Harness now owns agent context reporting
- **Web UI** — Next.js frontend with React Query SSE integration (`web/`)
