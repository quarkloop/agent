# Quark Deployment

Quark vNext treats process lifecycle as an operator concern. The supervisor
owns catalogs, spaces, and the embedded NATS hub, but it does not spawn runtime
or service processes.

## Docker Compose

Use the Compose stack from the repository root:

```bash
docker compose -f deploy/compose/quark.yml up supervisor
docker compose -f deploy/compose/quark.yml --profile runtime up runtime
docker compose -f deploy/compose/quark.yml --profile services up io space document runstate
```

Knowledge services require the `knowledge` profile:

```bash
docker compose -f deploy/compose/quark.yml --profile knowledge up dgraph gateway indexer
```

The DevOps profile uses a dedicated execution image because its typed build,
test, release, and container service functions require the Go toolchain and
Docker client in the running service process:

```bash
docker compose -f deploy/compose/quark.yml --profile devops up devops gateway
```

When a DevOps or file service operates on a bind-mounted workspace, set
`QUARK_WORKSPACE_CONTAINER_USER` to the workspace owner's numeric `UID:GID`.
The DevOps image stores Go home/module/build caches below ephemeral `/tmp`
locations so running builds and tests does not create product state or
root-owned cache files in that workspace.

Set `QUARK_GATEWAY_MAX_EXTERNAL_REQUESTS` when an environment requires a
hard bound on outbound model and embedding requests. Gateway enforces that
bound before provider dispatch; E2E passes its declared provider budget
through this setting.

Observability and infrastructure profiles are split so local development can
start only what it needs:

```bash
docker compose -f deploy/compose/quark.yml --profile observability up vector victoria-metrics
docker compose -f deploy/compose/quark.yml --profile secrets up openbao
docker compose -f deploy/compose/quark.yml --profile workflow up temporal
```

## systemd

Install binaries into `/usr/local/bin`, create a `quark` user, copy the unit
templates into `/etc/systemd/system`, and copy/edit environment examples under
`/etc/quark`.

```bash
sudo systemctl enable --now quark-supervisor.service
sudo systemctl enable --now quark-runtime@default.service
sudo systemctl enable --now quark-service@io.service
```

Install the bundled plugin directory under `/opt/quark/plugins`; supervisor
installs the required `quark-main` agent plugin in every newly created space
from that bundle. Runtime and service unit instances read their own environment
files. This keeps per-space runtime configuration and per-service arguments
outside the supervisor process.
