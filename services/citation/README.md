# Citation Service

`services/citation` is Quark Knowledge's mechanical grounding service. It
normalizes source spans, creates citation records, verifies claim overlap
against provided evidence, scores citation coverage, and renders source
references. It does not call LLMs, query the indexer, parse documents, or decide
which claims matter.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `citation_ResolveSpans` | `quark.citation.v1.CitationService/ResolveSpans` | `ResolveSpansRequest` | `ResolveSpansResponse` | Resolve selected text or hints into source offsets and confidence. |
| `citation_CreateCitation` | `quark.citation.v1.CitationService/CreateCitation` | `CreateCitationRequest` | `CitationSpan` | Create one normalized citation span from source text. |
| `citation_VerifyGrounding` | `quark.citation.v1.CitationService/VerifyGrounding` | `VerifyGroundingRequest` | `VerifyGroundingResponse` | Verify whether selected claims are mechanically supported by citation text spans. |
| `citation_ScoreCoverage` | `quark.citation.v1.CitationService/ScoreCoverage` | `ScoreCoverageRequest` | `ScoreCoverageResponse` | Score grounding coverage across a claim set. |
| `citation_RenderReferences` | `quark.citation.v1.CitationService/RenderReferences` | `RenderReferencesRequest` | `RenderReferencesResponse` | Render normalized source references for answers and artifacts. |

## Ownership Boundaries

- The agent owns semantic claim selection, answer composition, and deciding
  whether evidence is sufficient for the user.
- The document service owns extraction of raw source text and layout.
- The indexer owns storage and retrieval of canonical knowledge records.
- The citation service only checks evidence supplied in the request.

## Configuration

- `--addr`: gRPC listen address, default `127.0.0.1:7309`.
- `--skill-dir`: directory containing the service plugin `SKILL.md`.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.citation.v1.CitationService`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.

## Audit Notes

- The service is deterministic and request-local.
- There is no storage and no service-to-service client.
- Grounding verification is mechanical token coverage, not a semantic truth
  judgment. The agent must still reason about whether the evidence answers the
  user's question.
