# Security Policy

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report security issues via [GitHub's private security advisory](https://github.com/quarkloop/agent/security/advisories/new). We will acknowledge your report within 72 hours and work with you on a fix before any public disclosure.

## Scope

In scope:
- Supervisor (`supervisor/`) — control plane, embedded NATS, catalogs, spaces, sessions
- Runtime (`runtime/`) — agent loop, tool execution, service-function dispatch
- CLI (`cli/`) — NATS client for supervisor/runtime operations
- Services (`services/`) — typed NATS service-function owners (gateway, indexer, document, etc.)
- Plugins (`plugins/`) — installable tool, service, and agent extensions
- Web UI (`web/`) — Next.js frontend

Out of scope:
- Vulnerabilities in third-party LLM providers (Anthropic, OpenAI, OpenRouter)
- Issues in the web UI's npm dependencies unrelated to Quark's own code

## API Keys

Quark never stores API keys: they are injected at runtime from environment
variables and resolved only for spaces whose service-owned `space.json`
configuration declares the corresponding environment reference. Never commit
your `.env` file; use `.env.example` as a template.
