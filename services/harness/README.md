# Harness Service

`services/harness` is the Rust implementation of Quark's model-context
boundary. It packages plugin-owned prompt material and runtime facts into
bounded model messages, stores redacted context reports, and owns explicit
agent-authored memory records. Context reports separately identify explicitly
included memory material. It does not call models, tools, or other services.

## Build And Test

```bash
cargo fmt --manifest-path services/harness/Cargo.toml --check
cargo test --manifest-path services/harness/Cargo.toml
cargo clippy --manifest-path services/harness/Cargo.toml --all-targets -- -D warnings
```

## Service Functions

| Function | Responsibility |
| --- | --- |
| `harness_ComposeContext` | Package supplied material and persist a context report. |
| `harness_GetContextReport` | Retrieve one context report. |
| `harness_StreamContextReports` | Stream recent reports for one session. |
| `harness_PutMemory` | Persist an explicit memory record with provenance. |
| `harness_GetMemory` | Retrieve one memory record. |
| `harness_SearchMemory` | Search explicit memory within one scope. |
| `harness_DeleteMemory` | Remove one memory record. |

The responder uses the NATS service subjects `svc.harness.v1.<function>` and
queue group `q.service.v1.harness`, implementing the versioned NATSKit
envelope contract in Rust. Go-to-Rust compatibility is tested from
`runtime/pkg/harnessclient`.
