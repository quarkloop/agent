# tool-fs

Filesystem operations for agent use. Replaces the legacy `read` and `write` tools.

## Commands

- `fs read <path> [--start-line N] [--end-line N] --json` — Read file contents, optionally with line range
- `fs extract_pdf <path> [--max-chars N] --json` — Extract text from a PDF file with `pdftotext`
- `fs write <path> <content> --approved --json` — Write content to file (overwrite) after explicit user approval
- `fs append <path> <content> --approved --json` — Append content to file after explicit user approval
- `fs replace <path> <find> <replace-with> --approved --json` — Replace all occurrences of text after explicit user approval
- `fs list [path] [--recursive] [--include-hash] --json` — List directory (default: current directory)
- `fs stat <path> [--include-hash] --json` — Get file metadata and optional sha256
- `fs rm <path> --approved --json` — Remove file or directory after explicit user approval

## HTTP

All commands map to `POST /<command>` with JSON body.

## Important

- `write`, `append`, `replace`, and `rm` require `approved=true` after explicit user approval
- `write` overwrites existing files when approved
- `rm` removes files and directories permanently when approved
- `read` supports `--start-line` and `--end-line` for partial reads (1-based, inclusive)
- `extract_pdf` requires the `pdftotext` binary on `PATH`; set `--max-chars 0` when the agent needs the full extracted text
- Use absolute paths when possible
- For directory indexing, use `list`/`stat`/`read` or `extract_pdf` in place.
  Do not rename, restructure, write sidecars, or remove files unless the user
  explicitly approves that separate workspace-organization action.
- Use `include_hash` plus `modified` timestamps to track source identity for
  re-indexing without writing sidecar files into the user's directory.
