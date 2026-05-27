# Space Configuration

`space.json` is the authoritative configuration record for a space. It is
persisted by the Space service under its configured storage root. The CLI and
supervisor exchange this configuration through NATS contracts; Quark does not
create hidden configuration files in a user's working directory.

Agent and service plugin defaults remain in their plugin manifests. A space
configuration identifies the workspace and narrows installed plugins,
services, models, and permissions for that space.

## Knowledge Space

```json
{
  "schema": "quark.space/v1",
  "name": "knowledge-local",
  "version": "0.1.0",
  "working_dir": "/work/documents",
  "created_at": "2026-05-25T00:00:00Z",
  "updated_at": "2026-05-25T00:00:00Z",
  "model": {
    "provider": "openrouter",
    "name": "openrouter/owl-alpha",
    "env": ["OPENROUTER_API_KEY"]
  },
  "plugins": [
    {"ref": "quark/agent-quark-knowledge"},
    {"ref": "quark/service-document"},
    {"ref": "quark/service-gateway"},
    {"ref": "quark/service-indexer"},
    {"ref": "quark/service-citation"}
  ],
  "agents": [
    {"profile": "quark-knowledge", "enabled": true}
  ],
  "services": [
    {"name": "document", "ref": "quark/service-document"},
    {"name": "gateway", "ref": "quark/service-gateway"},
    {"name": "indexer", "ref": "quark/service-indexer"},
    {"name": "citation", "ref": "quark/service-citation"}
  ]
}
```

Gateway owns provider implementation and embedding model configuration. The
space record contains references and narrowing only; credentials remain
environment-injected and are never persisted in this file.

## Permission Narrowing

Installed agent profiles declare their maximum allowed service functions and
tools. A space may narrow that set:

```json
{
  "agents": [
    {
      "profile": "quark-system",
      "enabled": true,
      "services": ["system_Snapshot", "system_GetMetrics", "system_GetDiskUsage"]
    }
  ]
}
```

Supervisor rejects any override that grants access beyond the installed
profile maximum before issuing a runtime catalog.
