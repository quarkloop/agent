# service-memory

The memory service stores explicit space-scoped memory records. It is not a
hidden inference system and does not infer memories from raw prompts.

## Agent Workflows

1. Call `memory_Put` only when the agent has decided an explicit durable memory
   should be saved with provenance.
2. Call `memory_Get` or `memory_Search` when the current task needs known
   space-level memory.
3. Call `memory_Delete` only when the user has requested or approved deletion.

## RPCs

- `Put(PutRequest) -> PutResponse`
  - Generated service function: `memory_Put`
- `Get(GetRequest) -> GetResponse`
  - Generated service function: `memory_Get`
- `Search(SearchRequest) -> SearchResponse`
  - Generated service function: `memory_Search`
- `Delete(DeleteRequest) -> DeleteResponse`
  - Generated service function: `memory_Delete`

## Contract Notes

- The agent owns the decision to remember, retrieve, or forget.
- The service stores records and provenance only. It does not call the model
  service, indexer, citation service, or any other service.
- Deletion is approval-gated because it mutates durable knowledge.
