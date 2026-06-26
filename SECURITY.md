# Security Policy

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report security issues via [GitHub's private security advisory](https://github.com/quarkloop/agent/security/advisories/new). We will acknowledge your report within 72 hours and work with you on a fix before any public disclosure.

## Scope

In scope:
- Agent runtime (`agent/`)
- API server (`api-server/`)
- CLI (`cli/`)
- Tool binaries (`tools/`)

Out of scope:
- Vulnerabilities in third-party LLM providers (Anthropic, OpenAI, OpenRouter, Zhipu)
- Issues in the web UI's npm dependencies unrelated to Quark's own code

## API Keys

Quark never stores API keys: they are injected at runtime from environment
variables and resolved only for spaces whose service-owned `space.json`
configuration declares the corresponding environment reference. Never commit
your `.env` file; use `.env.example` as a template.
