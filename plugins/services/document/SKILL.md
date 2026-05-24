# service-document

The document service performs mechanical document extraction through NATS service functions. It is
for type detection, byte parsing, raw text extraction, layout extraction, page
records, table extraction, image extraction, and OCR.

## Agent Workflows

Use document service functions when a file needs parser or OCR support beyond
plain filesystem reads.

1. Call `document_DetectType` or `document_ParseBytes` to identify the source
   mechanically.
2. Call `document_ExtractText`, `document_GetPages`, `document_ExtractLayout`,
   `document_ExtractTables`, `document_ExtractImages`, or `document_RunOCR` as
   needed for source evidence.
3. Keep semantic work in the agent LLM loop: classify the document, infer an
   extraction schema, normalize fields, choose chunks, extract facts, extract
   entities and relations, and select citations.
4. Pass agent-produced semantic records to Gateway embedding and indexer service
   functions. Do not ask the document service to index or answer.

## RPCs

- `DetectType(DetectTypeRequest) -> DetectTypeResponse`
  - Generated service function: `document_DetectType`
- `ParseBytes(ParseBytesRequest) -> ParseBytesResponse`
  - Generated service function: `document_ParseBytes`
- `ExtractText(ExtractTextRequest) -> ExtractTextResponse`
  - Generated service function: `document_ExtractText`
  - Returns source/page provenance; runtime exposes opaque content/page
    references for subsequent Gateway and indexer calls.
- `ExtractLayout(ExtractLayoutRequest) -> ExtractLayoutResponse`
  - Generated service function: `document_ExtractLayout`
- `GetPages(GetPagesRequest) -> GetPagesResponse`
  - Generated service function: `document_GetPages`
- `ExtractTables(ExtractTablesRequest) -> ExtractTablesResponse`
  - Generated service function: `document_ExtractTables`
- `ExtractImages(ExtractImagesRequest) -> ExtractImagesResponse`
  - Generated service function: `document_ExtractImages`
  - Runtime converts media bytes into opaque image references; never copy
    binary content into a prompt.
- `RunOCR(RunOCRRequest) -> RunOCRResponse`
  - Generated service function: `document_RunOCR`

## Contract Notes

- The service returns source evidence only. It does not call LLMs.
- The service does not create embeddings, facts, entities, relations,
  citations, chunks, or index records.
- Large bytes should flow through approved runtime artifacts or content
  references when available. Do not paste binary data into prompts.
