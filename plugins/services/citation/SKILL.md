# service-citation

The citation service resolves source spans and verifies grounding. It is a
mechanical evidence service, not an answer generator.

## Agent Workflows

1. After the LLM selects facts, claims, and candidate evidence, call
   `citation_ResolveSpans` to normalize source spans and offsets.
2. Use `citation_CreateCitation` when a single selected evidence phrase needs a
   normalized citation record.
3. Before answering or indexing high-value claims, call
   `citation_VerifyGrounding` when grounding needs mechanical verification.
4. Use `citation_ScoreCoverage` to summarize grounding coverage across a set
   of claims before a final answer or index write.
5. Use `citation_RenderReferences` to render source references for user-facing
   answers and manual verification artifacts.
6. Pass resolved citations to `indexer_UpsertChunk.citations`,
   `indexer_UpsertFact.fact.citations`, and `provenance`.

## RPCs

- `ResolveSpans(ResolveSpansRequest) -> ResolveSpansResponse`
  - Generated service function: `citation_ResolveSpans`
- `CreateCitation(CreateCitationRequest) -> CitationSpan`
  - Generated service function: `citation_CreateCitation`
- `VerifyGrounding(VerifyGroundingRequest) -> VerifyGroundingResponse`
  - Generated service function: `citation_VerifyGrounding`
- `ScoreCoverage(ScoreCoverageRequest) -> ScoreCoverageResponse`
  - Generated service function: `citation_ScoreCoverage`
- `RenderReferences(RenderReferencesRequest) -> RenderReferencesResponse`
  - Generated service function: `citation_RenderReferences`

## Contract Notes

- The agent chooses claims and evidence candidates.
- This service does not call LLMs, query the indexer, or generate final
  answers.
- Verification results are evidence diagnostics; the agent decides how to use
  them in final reasoning.
