# Citation Service Plugin

The citation service owns source span resolution and grounding verification.
It does not choose which claims matter and does not generate user answers.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `citation_ResolveSpans` | `quark.citation.v1.CitationService/ResolveSpans` | read | no | yes | Resolve selected claim text into source spans with offsets and confidence. |
| `citation_VerifyGrounding` | `quark.citation.v1.CitationService/VerifyGrounding` | read | no | yes | Verify that provided citation spans ground selected claims. |

## Boundary

LLM semantic extraction remains in the agent loop. The agent selects candidate
facts and citations; this service normalizes and checks evidence. Indexer
stores the agent-produced facts, entities, relations, citations, embeddings,
and provenance.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.citation.v1.CitationService`.
- Required readiness: yes, before runtime receives this service in the catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing `QUARK_CITATION_ADDR`, failed health
  checks, descriptor version mismatch, and missing RPC descriptors.
