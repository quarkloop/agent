# System Service Plugin

The System service plugin exposes selected Linux/system service functions for
Quark System. It provides typed observations and approval-gated mutation plans;
it does not expose arbitrary shell execution or raw osquery SQL to agents.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `system_Snapshot` | `quark.system.v1.SystemService/Snapshot` | read | no | yes | Return a scoped snapshot of OS, kernel, uptime, mounts, disks, processes, network, and metrics. |
| `system_GetOSInfo` | `quark.system.v1.SystemService/GetOSInfo` | read | no | yes | Return operating-system identity and architecture. |
| `system_GetKernelInfo` | `quark.system.v1.SystemService/GetKernelInfo` | read | no | yes | Return kernel name, release, version, and machine architecture. |
| `system_GetUptime` | `quark.system.v1.SystemService/GetUptime` | read | no | yes | Return uptime and boot timestamp. |
| `system_ListPackages` | `quark.system.v1.SystemService/ListPackages` | read | no | yes | List installed packages from an approved backend. |
| `system_ListServices` | `quark.system.v1.SystemService/ListServices` | read | no | yes | List service manager units and their states. |
| `system_ListUsers` | `quark.system.v1.SystemService/ListUsers` | read | no | yes | List local users with optional system-user filtering. |
| `system_ListMounts` | `quark.system.v1.SystemService/ListMounts` | read | no | yes | List mounted filesystems and mount options. |
| `system_GetDiskUsage` | `quark.system.v1.SystemService/GetDiskUsage` | read | no | yes | Return disk usage for one path or known mounted filesystems. |
| `system_ListProcesses` | `quark.system.v1.SystemService/ListProcesses` | read | no | yes | List running processes with resource summaries. |
| `system_ListPorts` | `quark.system.v1.SystemService/ListPorts` | read | no | yes | List listening local ports with owning process when available. |
| `system_ListNetworkConnections` | `quark.system.v1.SystemService/ListNetworkConnections` | read | no | yes | List network connections by endpoint, state, and owning process when available. |
| `system_ReadLogs` | `quark.system.v1.SystemService/ReadLogs` | read | no | yes | Read a bounded tail from approved log sources. |
| `system_GetMetrics` | `quark.system.v1.SystemService/GetMetrics` | read | no | yes | Return load, memory, and swap metrics. |
| `system_KillProcess` | `quark.system.v1.SystemService/KillProcess` | admin | yes | no | Prepare an approval-gated process termination plan. |
| `system_RestartService` | `quark.system.v1.SystemService/RestartService` | admin | yes | no | Prepare an approval-gated service restart plan. |

## Backend Decision

The first implementation should use small native Linux readers for stable
surfaces such as `/etc/os-release`, `/proc`, mount information, and filesystem
usage. osquery can be useful for packages, services, users, processes, sockets,
and richer inventory, but it must be an optional backend wrapped by these typed
functions. Quark must not expose arbitrary osquery SQL to agents.

Current osquery packaging is dual Apache-2.0/GPL-2.0-only in upstream/package
metadata, so bundling or vendoring requires a dedicated license and packaging
audit. Until that audit is complete, Quark System may call an installed osquery
binary only through an approved adapter that maps selected queries to these
service functions.

## Approval

Read-only observations do not require approval. `system_KillProcess` and
`system_RestartService` are admin-risk functions and must go through runtime
approval before execution. The service should return a mutation plan with risks;
Core/runtime owns approval state and audit persistence.

## Non-Goals

- No full clone of shell, systemd, ps, netstat, journalctl, or osquery.
- No direct service-to-service calls.
- No unrestricted user directory mutation.
- No raw credentials, secrets, or full log dumps in responses.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.system.v1.SystemService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics should cover unavailable Linux readers, disabled optional
  osquery backend, missing descriptor RPCs, and unsupported service version.
