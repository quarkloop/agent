# Document Service

The document service owns mechanical document inspection and extraction. It is a
NATS service-function provider used by agents when source files need parser-backed evidence before
the agent performs semantic extraction through Gateway.

The service does not classify business meaning, infer schemas, choose chunks,
create embeddings, write index records, call another service, or answer users.

## Service Functions

| Service function | RPC method | Purpose |
| --- | --- | --- |
| `document_DetectType` | `quark.document.v1.DocumentService/DetectType` | Detect MIME type, extension, coarse document family, and source metadata. |
| `document_ParseBytes` | `quark.document.v1.DocumentService/ParseBytes` | Parse bytes or a file URI into mechanical metadata such as source hash, page count, and text availability. |
| `document_ExtractText` | `quark.document.v1.DocumentService/ExtractText` | Extract raw text with typed source and per-page provenance. |
| `document_ExtractLayout` | `quark.document.v1.DocumentService/ExtractLayout` | Return mechanical page layout blocks and bounding boxes. |
| `document_GetPages` | `quark.document.v1.DocumentService/GetPages` | Return page records with text, layout blocks, tables, and images. |
| `document_ExtractTables` | `quark.document.v1.DocumentService/ExtractTables` | Extract mechanically detected table rows. |
| `document_ExtractImages` | `quark.document.v1.DocumentService/ExtractImages` | Return typed image media for runtime-managed references. |
| `document_RunOCR` | `quark.document.v1.DocumentService/RunOCR` | Return text-layer pages as OCR-equivalent output, or fail clearly when an OCR backend is required but unavailable. |

## Supported Sources

- `content`: request bytes for small sources.
- `source_uri`: local path or `file://` URI owned by the caller.

## Supported Formats

- PDF text extraction through a configured `pdftotext` executable.
- Markdown and UTF-8 plain text.
- Image type detection and typed image source records; runtime keeps raw bytes
  behind opaque references before presenting results to an agent.

Receipts, CVs, papers, catalogs, certificates, and other domain-specific
formats are handled by the agent and Gateway service using the mechanical records
returned here. This service intentionally exposes format adapters without
encoding semantic schemas into the parser.

## Run

```bash
go run ./services/document/cmd/document --skill-dir plugins/services/document
```

Set `QUARK_PDFTOTEXT_PATH` or pass `--pdftotext` to select a specific PDF text
backend. When empty, the service resolves `pdftotext` from `PATH`.
