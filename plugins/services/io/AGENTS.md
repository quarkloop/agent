# io service agent boundaries

The `io` service owns mechanical host I/O only: files, shell commands, web search, and HTTP fetch.

## Redlines

- Do not call other Quark services from `services/io`.
- Do not parse documents semantically, embed text, or write to the indexer.
- Do not add LLM calls in this service.

## Packages

- `internal/iofs` — filesystem
- `internal/ioshell` — `bash -c` execution
- `internal/iosearch` — Brave/SerpAPI search
- `internal/iofetch` — bounded HTTP fetch with SSRF guards

Avoid naming Go packages `io` (stdlib collision); use `iosvc`, `iofs`, etc.

## Approval

Mutation RPCs and `Execute` require `approved: true` in the request. Runtime assistive mode also gates `approval_required` functions via the execution gate.

## Environment

- `QUARK_NATS_URL` — NATS endpoint used by the service-function transport
- `BRAVE_API_KEY` / `SERPAPI_KEY` — web search providers

## Fetch safety

`Fetch` allows http/https only, blocks private/link-local targets, caps body size and redirects, and returns transport errors in the response `error` field.
