# Quarkfile Examples

`Quarkfile` is the space-level configuration layer. Supervisor validates it,
resolves installed agent/tool/service/provider plugins, narrows permissions,
checks services, and passes versioned catalogs to runtime.

The examples below are intentionally small. They show the shape of each launch
profile without hiding the service/plugin contract in prompts.

## Knowledge With Local Embeddings

```yaml
quark: "1.0"
meta:
  name: knowledge-local
  version: "0.1.0"
model:
  provider: openrouter
  name: openai/gpt-4o-mini
  env:
    - OPENROUTER_API_KEY
plugins:
  - ref: quark/agent-quark-knowledge
  - ref: quark/service-io
  - ref: quark/service-core
  - ref: quark/service-model
  - ref: quark/service-document
  - ref: quark/service-ingestion
  - ref: quark/service-embedding
  - ref: quark/service-indexer
  - ref: quark/service-citation
agents:
  - profile: quark-knowledge
    enabled: true
services:
  - name: io
    ref: quark/service-io
    mode: local
    address_env: QUARK_IO_ADDR
  - name: core
    ref: quark/service-core
    mode: local
    address_env: QUARK_CORE_ADDR
  - name: model
    ref: quark/service-model
    mode: local
    address_env: QUARK_MODEL_SERVICE_ADDR
  - name: document
    ref: quark/service-document
    mode: local
    address_env: QUARK_DOCUMENT_ADDR
  - name: ingestion
    ref: quark/service-ingestion
    mode: local
    address_env: QUARK_INGESTION_ADDR
  - name: embedding
    ref: quark/service-embedding
    mode: local
    address_env: QUARK_EMBEDDING_ADDR
  - name: indexer
    ref: quark/service-indexer
    mode: local
    address_env: QUARK_INDEXER_ADDR
  - name: citation
    ref: quark/service-citation
    mode: local
    address_env: QUARK_CITATION_ADDR
embedding:
  service: embedding
  provider: local
  model: local-hash-v1
  dimensions: 32
```

## Knowledge With OpenRouter Embeddings

```yaml
embedding:
  service: embedding
  provider: openrouter
  model: nvidia/llama-nemotron-embed-vl-1b-v2:free
  dimensions: 2048
```

Set the provider credential before supervisor starts:

```bash
export OPENROUTER_API_KEY=sk-or-v1-...
```

## DevOps

```yaml
quark: "1.0"
meta:
  name: devops
  version: "0.1.0"
model:
  provider: openrouter
  name: openai/gpt-4o-mini
  env:
    - OPENROUTER_API_KEY
plugins:
  - ref: quark/agent-quark-devops
  - ref: quark/service-io
  - ref: quark/service-devops
  - ref: quark/service-build-release
agents:
  - profile: quark-devops
    enabled: true
services:
  - name: devops
    ref: quark/service-devops
    mode: local
    address_env: QUARK_DEVOPS_ADDR
  - name: build-release
    ref: quark/service-build-release
    mode: local
    address_env: QUARK_BUILD_RELEASE_ADDR
```

## System

```yaml
quark: "1.0"
meta:
  name: system
  version: "0.1.0"
model:
  provider: openrouter
  name: openai/gpt-4o-mini
  env:
    - OPENROUTER_API_KEY
plugins:
  - ref: quark/agent-quark-system
  - ref: quark/service-system
agents:
  - profile: quark-system
    enabled: true
services:
  - name: system
    ref: quark/service-system
    mode: local
    address_env: QUARK_SYSTEM_ADDR
```

## Permission Narrowing

Agent profiles declare maximum tool and service-function permissions. Quarkfile
overrides can narrow those permissions for a space, but cannot grant anything
outside the installed profile maximum.

```yaml
agents:
  - profile: quark-system
    enabled: true
    permissions:
      services:
        - system_Snapshot
        - system_GetMetrics
        - system_GetDiskUsage
```

An empty narrowed permission set denies all agent-callable tools and service
functions for that profile.
