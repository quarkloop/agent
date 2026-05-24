# IO Service

`services/io` implements Quark's mechanical host I/O service. It exposes the
`quark.io.v1.IOService` service-function contract for the NATS-native architecture and is the
single owner of filesystem, approved shell, bounded web fetch, web search, and
legacy PDF text extraction behavior.

## Service Functions

| Function | Responsibility |
| --- | --- |
| `io_Read` | Read a text file with optional line range. |
| `io_List` | List directory entries with optional recursion and hashes. |
| `io_Stat` | Return metadata and optional file hash. |
| `io_ReadMedia` | Read bounded media bytes and typed source metadata for runtime-managed references. |
| `io_ExtractPdf` | Extract plain PDF text through `pdftotext`; use document service functions for indexing workflows. |
| `io_SearchWeb` | Search via Brave or SerpAPI when configured. |
| `io_Fetch` | Fetch HTTP/HTTPS content with size, timeout, redirect, and private-network guards. |
| `io_Write` | Atomically overwrite a file after approval. |
| `io_Append` | Atomically append to a file after approval. |
| `io_Replace` | Atomically replace non-empty text matches after approval. |
| `io_Remove` | Remove a file or directory after approval. |
| `io_Execute` | Execute a non-empty shell command after approval. |

## Package Boundaries

- `internal/iofs`: filesystem reads and approved mutations.
- `internal/ioshell`: approved `bash -c` execution.
- `internal/iofetch`: bounded HTTP/HTTPS fetch with SSRF guards.
- `internal/iosearch`: Brave/SerpAPI search adapters.
- `internal/iosvc`: transport boundary mapping between protobuf DTOs and service
  package requests/results.

The service does not call other Quark services. It does not perform semantic
document extraction, embedding, indexing, or LLM calls.
