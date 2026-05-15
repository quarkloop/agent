# service-citation

The citation service resolves source spans and verifies grounding. It is a
mechanical evidence service, not an answer generator.

## Agent Workflows

1. After the LLM selects facts, claims, and candidate evidence, call
   `citation_ResolveSpans` to normalize source spans and offsets.
2. Before answering or indexing high-value claims, call
   `citation_VerifyGrounding` when grounding needs mechanical verification.
3. Pass resolved citations to `indexer_IndexDocument.citations`,
   `facts.citations`, and `provenance`.

## RPCs

- `ResolveSpans(ResolveSpansRequest) -> ResolveSpansResponse`
  - Generated service function: `citation_ResolveSpans`
- `VerifyGrounding(VerifyGroundingRequest) -> VerifyGroundingResponse`
  - Generated service function: `citation_VerifyGrounding`

## Contract Notes

- The agent chooses claims and evidence candidates.
- This service does not call LLMs, query the indexer, or generate final
  answers.
- Verification results are evidence diagnostics; the agent decides how to use
  them in final reasoning.
