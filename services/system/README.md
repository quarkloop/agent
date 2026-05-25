# System Service

`services/system` is the planned Quark System service boundary. It owns
selected Linux/system observations and approval-gated mutation plans. It must
stay narrow and typed: agents coordinate user intent; this service reads system
state or prepares explicit mutation plans.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `system_Snapshot` | `quark.system.v1.SystemService/Snapshot` | `SnapshotRequest` | `SnapshotResponse` | Return a scoped snapshot of OS, kernel, uptime, mounts, disks, processes, network, and metrics. |
| `system_GetOSInfo` | `quark.system.v1.SystemService/GetOSInfo` | `GetOSInfoRequest` | `GetOSInfoResponse` | Return operating-system identity and architecture. |
| `system_GetKernelInfo` | `quark.system.v1.SystemService/GetKernelInfo` | `GetKernelInfoRequest` | `GetKernelInfoResponse` | Return kernel name, release, version, and machine architecture. |
| `system_GetUptime` | `quark.system.v1.SystemService/GetUptime` | `GetUptimeRequest` | `GetUptimeResponse` | Return uptime and boot timestamp. |
| `system_ListPackages` | `quark.system.v1.SystemService/ListPackages` | `ListPackagesRequest` | `ListPackagesResponse` | List installed packages from an approved backend. |
| `system_ListServices` | `quark.system.v1.SystemService/ListServices` | `ListServicesRequest` | `ListServicesResponse` | List service manager units and their states. |
| `system_ListUsers` | `quark.system.v1.SystemService/ListUsers` | `ListUsersRequest` | `ListUsersResponse` | List local users with optional system-user filtering. |
| `system_ListMounts` | `quark.system.v1.SystemService/ListMounts` | `ListMountsRequest` | `ListMountsResponse` | List mounted filesystems and mount options. |
| `system_GetDiskUsage` | `quark.system.v1.SystemService/GetDiskUsage` | `GetDiskUsageRequest` | `GetDiskUsageResponse` | Return disk usage for one path or known mounted filesystems. |
| `system_ListProcesses` | `quark.system.v1.SystemService/ListProcesses` | `ListProcessesRequest` | `ListProcessesResponse` | List running processes with resource summaries. |
| `system_ListPorts` | `quark.system.v1.SystemService/ListPorts` | `ListPortsRequest` | `ListPortsResponse` | List listening local ports with owning process when available. |
| `system_ListNetworkConnections` | `quark.system.v1.SystemService/ListNetworkConnections` | `ListNetworkConnectionsRequest` | `ListNetworkConnectionsResponse` | List network connections by endpoint, state, and owner when available. |
| `system_ReadLogs` | `quark.system.v1.SystemService/ReadLogs` | `ReadLogsRequest` | `ReadLogsResponse` | Read a bounded tail from approved log sources. |
| `system_GetMetrics` | `quark.system.v1.SystemService/GetMetrics` | `GetMetricsRequest` | `GetMetricsResponse` | Return load, memory, and swap metrics. |
| `system_KillProcess` | `quark.system.v1.SystemService/KillProcess` | `KillProcessRequest` | `KillProcessResponse` | Prepare an approval-gated process termination plan. |
| `system_RestartService` | `quark.system.v1.SystemService/RestartService` | `RestartServiceRequest` | `RestartServiceResponse` | Prepare an approval-gated service restart plan. |

## NATS Subjects

Each function is registered by `pkg/natskit` using its canonical subject:
`svc.system.v1.<snake_case_function>`. For example, `system_Snapshot` is
`svc.system.v1.snapshot`, `system_GetMetrics` is
`svc.system.v1.get_metrics`, and `system_RestartService` is
`svc.system.v1.restart_service`. Descriptor subjects are authoritative.

## Ownership Boundaries

- Quark System agent decides what to inspect and explains observations.
- System service reads selected OS state and prepares mutation plans.
- Core/runtime owns approval state, audit persistence, and execution gating.
- The service does not call model, indexer, space, or other services.
- `pkg/natskit` owns NATS registration and response envelopes; System owns
  typed adapters, observation shaping, and mutation-plan policy.

## Backend Notes

Native Linux readers should cover stable initial functions. osquery is a useful
candidate for package, service, process, socket, user, and inventory queries,
but only as an optional backend behind typed service functions. Do not expose
raw SQL to agents.

Upstream/package metadata currently reports osquery under Apache-2.0 or
GPL-2.0-only licensing, so vendoring or bundling requires a dedicated license
and packaging review. Until then, use an installed binary adapter only when the
user environment provides it.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.system.v1.SystemService`.
- Descriptor source: service plugin manifest and NATS service metadata.
- Readiness requires descriptor registration and available mandatory native
  Linux readers.
