package systemsvc

import (
	"context"
	"strings"

	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Server) Snapshot(_ context.Context, req *systemv1.SnapshotRequest) (*systemv1.SnapshotResponse, error) {
	osInfo, err := s.host.OSInfo()
	if err != nil {
		return nil, observationError(err)
	}
	kernel, err := s.host.KernelInfo()
	if err != nil {
		return nil, observationError(err)
	}
	uptime, err := s.host.Uptime()
	if err != nil {
		return nil, observationError(err)
	}
	mounts, err := s.host.Mounts()
	if err != nil {
		return nil, observationError(err)
	}
	resp := &systemv1.SnapshotResponse{
		Os:         osInfo,
		Kernel:     kernel,
		Uptime:     uptime,
		Mounts:     mounts,
		Disks:      &systemv1.GetDiskUsageResponse{Disks: s.host.DiskUsageForMounts(mounts)},
		ObservedAt: timestamppb.Now(),
	}
	if req.GetIncludeProcesses() {
		resp.Processes, err = s.host.Processes(limitOrDefault(25), "")
		if err != nil {
			return nil, observationError(err)
		}
	}
	if req.GetIncludeNetwork() {
		resp.Ports, err = s.network.Ports("", limitOrDefault(50))
		if err != nil {
			return nil, observationError(err)
		}
		resp.NetworkConnections, err = s.network.Connections("", limitOrDefault(50))
		if err != nil {
			return nil, observationError(err)
		}
	}
	if req.GetIncludeMetrics() {
		resp.Metrics = s.host.Metrics()
	}
	return resp, nil
}

func (s *Server) GetOSInfo(context.Context, *systemv1.GetOSInfoRequest) (*systemv1.GetOSInfoResponse, error) {
	info, err := s.host.OSInfo()
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.GetOSInfoResponse{Os: info}, nil
}

func (s *Server) GetKernelInfo(context.Context, *systemv1.GetKernelInfoRequest) (*systemv1.GetKernelInfoResponse, error) {
	info, err := s.host.KernelInfo()
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.GetKernelInfoResponse{Kernel: info}, nil
}

func (s *Server) GetUptime(context.Context, *systemv1.GetUptimeRequest) (*systemv1.GetUptimeResponse, error) {
	uptime, err := s.host.Uptime()
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.GetUptimeResponse{Uptime: uptime}, nil
}

func (s *Server) ListPackages(ctx context.Context, req *systemv1.ListPackagesRequest) (*systemv1.ListPackagesResponse, error) {
	manager := strings.TrimSpace(req.GetManager())
	if manager == "" {
		manager = s.inventory.DefaultPackageManager()
	}
	packages, err := s.inventory.Packages(ctx, manager, limitOrDefault(int(req.GetLimit())))
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.ListPackagesResponse{Packages: packages}, nil
}

func (s *Server) ListServices(ctx context.Context, req *systemv1.ListServicesRequest) (*systemv1.ListServicesResponse, error) {
	services, err := s.inventory.Services(ctx, firstNonBlank(req.GetManager(), "systemd"), req.GetState(), limitOrDefault(int(req.GetLimit())))
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.ListServicesResponse{Services: services}, nil
}

func (s *Server) ListUsers(_ context.Context, req *systemv1.ListUsersRequest) (*systemv1.ListUsersResponse, error) {
	users, err := s.host.Users(req.GetIncludeSystem())
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.ListUsersResponse{Users: users}, nil
}

func (s *Server) ListMounts(context.Context, *systemv1.ListMountsRequest) (*systemv1.ListMountsResponse, error) {
	mounts, err := s.host.Mounts()
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.ListMountsResponse{Mounts: mounts}, nil
}

func (s *Server) GetDiskUsage(_ context.Context, req *systemv1.GetDiskUsageRequest) (*systemv1.GetDiskUsageResponse, error) {
	usage, err := s.host.DiskUsage(firstNonBlank(req.GetPath(), "/"))
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.GetDiskUsageResponse{Disks: []*systemv1.DiskUsage{usage}}, nil
}

func (s *Server) ListProcesses(_ context.Context, req *systemv1.ListProcessesRequest) (*systemv1.ListProcessesResponse, error) {
	processes, err := s.host.Processes(limitOrDefault(int(req.GetLimit())), req.GetFilter())
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.ListProcessesResponse{Processes: processes}, nil
}

func (s *Server) ListPorts(_ context.Context, req *systemv1.ListPortsRequest) (*systemv1.ListPortsResponse, error) {
	ports, err := s.network.Ports(req.GetProtocol(), limitOrDefault(int(req.GetLimit())))
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.ListPortsResponse{Ports: ports}, nil
}

func (s *Server) ListNetworkConnections(_ context.Context, req *systemv1.ListNetworkConnectionsRequest) (*systemv1.ListNetworkConnectionsResponse, error) {
	connections, err := s.network.Connections(req.GetState(), limitOrDefault(int(req.GetLimit())))
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.ListNetworkConnectionsResponse{Connections: connections}, nil
}

func (s *Server) ReadLogs(_ context.Context, req *systemv1.ReadLogsRequest) (*systemv1.ReadLogsResponse, error) {
	lines, err := s.logs.Read(req.GetSource(), int(req.GetTailLines()), req.GetFilter())
	if err != nil {
		return nil, observationError(err)
	}
	return &systemv1.ReadLogsResponse{Lines: lines}, nil
}

func (s *Server) GetMetrics(context.Context, *systemv1.GetMetricsRequest) (*systemv1.GetMetricsResponse, error) {
	return &systemv1.GetMetricsResponse{Metrics: s.host.Metrics()}, nil
}

func (s *Server) KillProcess(_ context.Context, req *systemv1.KillProcessRequest) (*systemv1.KillProcessResponse, error) {
	if req.GetPid() <= 0 {
		return nil, serviceerrors.InvalidArgument("pid is required")
	}
	return &systemv1.KillProcessResponse{Plan: s.planner.KillProcess(req.GetPid(), req.GetReason())}, nil
}

func (s *Server) RestartService(_ context.Context, req *systemv1.RestartServiceRequest) (*systemv1.RestartServiceResponse, error) {
	if strings.TrimSpace(req.GetName()) == "" {
		return nil, serviceerrors.InvalidArgument("name is required")
	}
	return &systemv1.RestartServiceResponse{Plan: s.planner.RestartService(req.GetName(), req.GetManager(), req.GetReason())}, nil
}
