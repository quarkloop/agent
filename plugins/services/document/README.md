# Document Service Plugin

The document service owns mechanical document inspection and extraction. It
does not perform semantic extraction, schema inference, chunk selection,
embedding, indexing, retrieval, or answer generation.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `document_DetectType` | `quark.document.v1.DocumentService/DetectType` | read | no | yes | Detect MIME type, extension, and coarse document family. |
| `document_ParseBytes` | `quark.document.v1.DocumentService/ParseBytes` | read | no | yes | Parse bytes into mechanical metadata: source hash, page count, and text availability. |
| `document_ExtractText` | `quark.document.v1.DocumentService/ExtractText` | read | no | yes | Extract raw text and per-page offsets. |
| `document_ExtractLayout` | `quark.document.v1.DocumentService/ExtractLayout` | read | no | yes | Extract layout blocks and bounding boxes. |
| `document_GetPages` | `quark.document.v1.DocumentService/GetPages` | read | no | yes | Return page records with text, layout, tables, and images. |
| `document_ExtractTables` | `quark.document.v1.DocumentService/ExtractTables` | read | no | yes | Extract detected tables as headers and cells. |
| `document_ExtractImages` | `quark.document.v1.DocumentService/ExtractImages` | read | no | yes | Extract image references, MIME types, and coordinates. |
| `document_RunOCR` | `quark.document.v1.DocumentService/RunOCR` | read | no | yes | Run OCR and return raw text with confidence. |

## Boundary

Mechanical extraction belongs here. LLM semantic extraction belongs in the
agent/runtime model path. The agent classifies documents, infers schemas,
normalizes fields, chooses chunks, extracts facts/entities/relations, selects
citations, and sends the resulting canonical records to embedding and indexer
service functions.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.document.v1.DocumentService`.
- Required readiness: yes, before runtime receives this service in the catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing `QUARK_DOCUMENT_ADDR`, failed health
  checks, descriptor version mismatch, and missing RPC descriptors.
