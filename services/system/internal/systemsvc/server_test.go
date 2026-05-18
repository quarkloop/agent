package systemsvc

import (
	"context"
	"runtime"
	"strings"
	"testing"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
)

func TestReadOnlySystemObservations(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("system service currently targets Linux data sources")
	}
	t.Parallel()

	server := NewServer()
	ctx := context.Background()

	osInfo, err := server.GetOSInfo(ctx, &systemv1.GetOSInfoRequest{})
	if err != nil {
		t.Fatalf("os info: %v", err)
	}
	if osInfo.GetOs().GetArchitecture() == "" {
		t.Fatalf("os info missing architecture: %+v", osInfo.GetOs())
	}

	kernel, err := server.GetKernelInfo(ctx, &systemv1.GetKernelInfoRequest{})
	if err != nil {
		t.Fatalf("kernel info: %v", err)
	}
	if kernel.GetKernel().GetRelease() == "" {
		t.Fatalf("kernel info missing release: %+v", kernel.GetKernel())
	}

	uptime, err := server.GetUptime(ctx, &systemv1.GetUptimeRequest{})
	if err != nil {
		t.Fatalf("uptime: %v", err)
	}
	if uptime.GetUptime().GetSeconds() <= 0 || uptime.GetUptime().GetBootTime() == nil {
		t.Fatalf("uptime response is incomplete: %+v", uptime.GetUptime())
	}

	mounts, err := server.ListMounts(ctx, &systemv1.ListMountsRequest{})
	if err != nil {
		t.Fatalf("mounts: %v", err)
	}
	if len(mounts.GetMounts()) == 0 {
		t.Fatal("expected at least one mount")
	}

	disk, err := server.GetDiskUsage(ctx, &systemv1.GetDiskUsageRequest{Path: "/"})
	if err != nil {
		t.Fatalf("disk usage: %v", err)
	}
	if len(disk.GetDisks()) != 1 || disk.GetDisks()[0].GetTotalBytes() == 0 {
		t.Fatalf("disk response is incomplete: %+v", disk.GetDisks())
	}

	processes, err := server.ListProcesses(ctx, &systemv1.ListProcessesRequest{Limit: 5})
	if err != nil {
		t.Fatalf("processes: %v", err)
	}
	if len(processes.GetProcesses()) == 0 || len(processes.GetProcesses()) > 5 {
		t.Fatalf("process limit not respected: %+v", processes.GetProcesses())
	}
	for i := 1; i < len(processes.GetProcesses()); i++ {
		if processes.GetProcesses()[i-1].GetPid() > processes.GetProcesses()[i].GetPid() {
			t.Fatalf("processes are not sorted by pid: %+v", processes.GetProcesses())
		}
	}

	metrics, err := server.GetMetrics(ctx, &systemv1.GetMetricsRequest{})
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if metrics.GetMetrics().GetMemoryTotalBytes() == 0 {
		t.Fatalf("metrics missing memory total: %+v", metrics.GetMetrics())
	}
}

func TestSnapshotIncludesRequestedSections(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("system service currently targets Linux data sources")
	}
	t.Parallel()

	resp, err := NewServer().Snapshot(context.Background(), &systemv1.SnapshotRequest{
		IncludeProcesses: true,
		IncludeNetwork:   true,
		IncludeMetrics:   true,
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if resp.GetObservedAt() == nil || resp.GetOs() == nil || resp.GetKernel() == nil || resp.GetUptime() == nil {
		t.Fatalf("snapshot missing required sections: %+v", resp)
	}
	if len(resp.GetProcesses()) == 0 || resp.GetMetrics() == nil {
		t.Fatalf("snapshot missing requested sections: %+v", resp)
	}
}

func TestSafeBackendsReturnBoundedResponses(t *testing.T) {
	t.Parallel()
	server := NewServer()
	ctx := context.Background()

	packages, err := server.ListPackages(ctx, &systemv1.ListPackagesRequest{Manager: "unsupported", Limit: 5})
	if err != nil {
		t.Fatalf("unsupported package manager should be empty, not error: %v", err)
	}
	if len(packages.GetPackages()) != 0 {
		t.Fatalf("unsupported package manager returned packages: %+v", packages.GetPackages())
	}

	services, err := server.ListServices(ctx, &systemv1.ListServicesRequest{Manager: "unsupported", Limit: 5})
	if err != nil {
		t.Fatalf("unsupported service manager should be empty, not error: %v", err)
	}
	if len(services.GetServices()) != 0 {
		t.Fatalf("unsupported service manager returned services: %+v", services.GetServices())
	}
}

func TestApprovedLogPathAndNetworkParsing(t *testing.T) {
	t.Parallel()

	if _, err := logPath("relative.log"); err == nil {
		t.Fatal("expected relative log source to be rejected")
	}
	if _, err := logPath("/tmp/not-a-system-log"); err == nil {
		t.Fatal("expected non-/var/log source to be rejected")
	}
	path, err := logPath("messages")
	if err != nil {
		t.Fatalf("messages alias should resolve: %v", err)
	}
	if path != "/var/log/messages" {
		t.Fatalf("messages path = %q", path)
	}

	addr, port := parseEndpoint("0100007F:1F90")
	if addr != "127.0.0.1" || port != 8080 {
		t.Fatalf("endpoint = %s:%d, want 127.0.0.1:8080", addr, port)
	}
	if tcpState("0A") != "LISTEN" || tcpState("01") != "ESTABLISHED" {
		t.Fatalf("tcp state mapping failed")
	}
}

func TestMutationFunctionsReturnApprovalPlans(t *testing.T) {
	t.Parallel()
	server := NewServer()

	kill, err := server.KillProcess(context.Background(), &systemv1.KillProcessRequest{Pid: 1234, Reason: "test plan"})
	if err != nil {
		t.Fatalf("kill plan: %v", err)
	}
	if !kill.GetPlan().GetApprovalRequired() || kill.GetPlan().GetAction() != "system.kill_process" || len(kill.GetPlan().GetRisks()) != 1 {
		t.Fatalf("kill plan missing approval metadata: %+v", kill.GetPlan())
	}

	restart, err := server.RestartService(context.Background(), &systemv1.RestartServiceRequest{Name: "quark.service", Reason: "test plan"})
	if err != nil {
		t.Fatalf("restart plan: %v", err)
	}
	if !restart.GetPlan().GetApprovalRequired() || restart.GetPlan().GetAction() != "system.restart_service" {
		t.Fatalf("restart plan missing approval metadata: %+v", restart.GetPlan())
	}

	if _, err := server.KillProcess(context.Background(), &systemv1.KillProcessRequest{}); err == nil {
		t.Fatal("expected kill process to require pid")
	}
	if _, err := server.RestartService(context.Background(), &systemv1.RestartServiceRequest{}); err == nil {
		t.Fatal("expected restart service to require name")
	}
}

func TestDescriptorMatchesSystemPluginContract(t *testing.T) {
	t.Parallel()
	descriptor := Descriptor("127.0.0.1:7311", &servicev1.SkillDescriptor{Name: "service-system"})

	if descriptor.GetName() != "system" || descriptor.GetType() != "system" {
		t.Fatalf("descriptor identity mismatch: %+v", descriptor)
	}
	functions := map[string]bool{}
	for _, rpc := range descriptor.GetRpcs() {
		if rpc.GetFunctionName() == "" {
			t.Fatalf("rpc has empty function name: %+v", rpc)
		}
		if !strings.HasPrefix(rpc.GetFunctionName(), "system_") {
			t.Fatalf("unexpected function name %q", rpc.GetFunctionName())
		}
		functions[rpc.GetFunctionName()] = true
	}
	for _, name := range []string{"system_Snapshot", "system_GetMetrics", "system_KillProcess", "system_RestartService"} {
		if !functions[name] {
			t.Fatalf("descriptor missing function %s", name)
		}
	}
}
