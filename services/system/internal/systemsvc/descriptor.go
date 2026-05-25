package systemsvc

import (
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	service := "quark.system.v1.SystemService"
	return &servicev1.ServiceDescriptor{
		Name:    "system",
		Type:    "system",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc("system_Snapshot", "snapshot", service, "Snapshot", "quark.system.v1.SnapshotRequest", "quark.system.v1.SnapshotResponse", "Return a scoped system snapshot."),
			rpc("system_GetOSInfo", "get_os_info", service, "GetOSInfo", "quark.system.v1.GetOSInfoRequest", "quark.system.v1.GetOSInfoResponse", "Return operating-system identity."),
			rpc("system_GetKernelInfo", "get_kernel_info", service, "GetKernelInfo", "quark.system.v1.GetKernelInfoRequest", "quark.system.v1.GetKernelInfoResponse", "Return kernel information."),
			rpc("system_GetUptime", "get_uptime", service, "GetUptime", "quark.system.v1.GetUptimeRequest", "quark.system.v1.GetUptimeResponse", "Return uptime and boot time."),
			rpc("system_ListPackages", "list_packages", service, "ListPackages", "quark.system.v1.ListPackagesRequest", "quark.system.v1.ListPackagesResponse", "List installed packages."),
			rpc("system_ListServices", "list_services", service, "ListServices", "quark.system.v1.ListServicesRequest", "quark.system.v1.ListServicesResponse", "List service manager units."),
			rpc("system_ListUsers", "list_users", service, "ListUsers", "quark.system.v1.ListUsersRequest", "quark.system.v1.ListUsersResponse", "List local users."),
			rpc("system_ListMounts", "list_mounts", service, "ListMounts", "quark.system.v1.ListMountsRequest", "quark.system.v1.ListMountsResponse", "List mounted filesystems."),
			rpc("system_GetDiskUsage", "get_disk_usage", service, "GetDiskUsage", "quark.system.v1.GetDiskUsageRequest", "quark.system.v1.GetDiskUsageResponse", "Return disk usage."),
			rpc("system_ListProcesses", "list_processes", service, "ListProcesses", "quark.system.v1.ListProcessesRequest", "quark.system.v1.ListProcessesResponse", "List processes."),
			rpc("system_ListPorts", "list_ports", service, "ListPorts", "quark.system.v1.ListPortsRequest", "quark.system.v1.ListPortsResponse", "List listening ports."),
			rpc("system_ListNetworkConnections", "list_network_connections", service, "ListNetworkConnections", "quark.system.v1.ListNetworkConnectionsRequest", "quark.system.v1.ListNetworkConnectionsResponse", "List network connections."),
			rpc("system_ReadLogs", "read_logs", service, "ReadLogs", "quark.system.v1.ReadLogsRequest", "quark.system.v1.ReadLogsResponse", "Read bounded logs."),
			rpc("system_GetMetrics", "get_metrics", service, "GetMetrics", "quark.system.v1.GetMetricsRequest", "quark.system.v1.GetMetricsResponse", "Return system metrics."),
			rpc("system_KillProcess", "kill_process", service, "KillProcess", "quark.system.v1.KillProcessRequest", "quark.system.v1.KillProcessResponse", "Prepare a process kill plan."),
			rpc("system_RestartService", "restart_service", service, "RestartService", "quark.system.v1.RestartServiceRequest", "quark.system.v1.RestartServiceResponse", "Prepare a service restart plan."),
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
}

func rpc(functionName, function, service, method, request, response, description string) *servicev1.RpcDescriptor {
	return &servicev1.RpcDescriptor{
		Service:      service,
		Method:       method,
		Request:      request,
		Response:     response,
		Description:  description,
		FunctionName: functionName,
		Owner:        "system",
		Subject:      "svc.system.v1." + function,
	}
}
