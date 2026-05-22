# IO Service Plugin

Declares the `io` gRPC service (`bin/io-service`). Implementation lives in
`services/io`. The service owns mechanical host I/O only: filesystem reads and
approved mutations, approved shell execution, bounded HTTP fetch, web search,
and legacy PDF text extraction.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `io_Read` | `quark.io.v1.IOService/Read` | read | no | yes | Read a text file, optionally with a 1-based inclusive line range. |
| `io_List` | `quark.io.v1.IOService/List` | read | no | yes | List directory entries with optional recursion and sha256 hashes. |
| `io_Stat` | `quark.io.v1.IOService/Stat` | read | no | yes | Return file metadata and optional sha256 for regular files. |
| `io_ExtractPdf` | `quark.io.v1.IOService/ExtractPdf` | read | no | yes | Extract PDF text with `pdftotext`; document service functions are preferred for knowledge indexing. |
| `io_SearchWeb` | `quark.io.v1.IOService/SearchWeb` | read | no | yes | Search the web through Brave or SerpAPI when API keys are configured. |
| `io_Fetch` | `quark.io.v1.IOService/Fetch` | read | no | yes | Fetch an HTTP/HTTPS URL with bounded size, timeout, redirect, and private-network guards. |
| `io_Write` | `quark.io.v1.IOService/Write` | write | yes | no | Atomically overwrite a file after explicit user approval. |
| `io_Append` | `quark.io.v1.IOService/Append` | write | yes | no | Atomically append to a file after explicit user approval. |
| `io_Replace` | `quark.io.v1.IOService/Replace` | write | yes | no | Atomically replace non-empty text matches after explicit user approval. |
| `io_Remove` | `quark.io.v1.IOService/Remove` | admin | yes | no | Remove a file or directory after explicit user approval. |
| `io_Execute` | `quark.io.v1.IOService/Execute` | admin | yes | no | Execute a non-empty shell command through `bash -c` after explicit user approval. |

## Configuration

- `QUARK_IO_ADDR`: service listen address used by the supervisor-resolved
  service catalog.
- `QUARK_PDFTOTEXT_PATH`: optional path to the `pdftotext` executable.
- `BRAVE_API_KEY` / `SERPAPI_KEY`: optional web search provider credentials.

## Boundaries

The IO service does not call LLMs, parse documents semantically, create
embeddings, write to the indexer, or call another Quark service. Runtime and
agents must use the generated service functions through the shared
service-function/tool-call surface.
