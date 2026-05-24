package systemsvc

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) Snapshot(ctx context.Context, req *systemv1.SnapshotRequest) (*systemv1.SnapshotResponse, error) {
	osInfo, _ := readOSInfo()
	kernel, _ := readKernelInfo()
	uptime, _ := readUptime()
	mounts, _ := readMounts()
	disks := diskUsageForMounts(mounts)
	resp := &systemv1.SnapshotResponse{
		Os:         osInfo,
		Kernel:     kernel,
		Uptime:     uptime,
		Mounts:     mounts,
		Disks:      &systemv1.GetDiskUsageResponse{Disks: disks},
		ObservedAt: timestamppb.Now(),
	}
	if req.GetIncludeProcesses() {
		resp.Processes = listProcesses(reqLimit(25), "")
	}
	if req.GetIncludeNetwork() {
		resp.Ports = listPorts("", reqLimit(50))
		resp.NetworkConnections = listConnections("", reqLimit(50))
	}
	if req.GetIncludeMetrics() {
		resp.Metrics = readMetrics()
	}
	_ = ctx
	return resp, nil
}

func (s *Server) GetOSInfo(context.Context, *systemv1.GetOSInfoRequest) (*systemv1.GetOSInfoResponse, error) {
	info, err := readOSInfo()
	if err != nil {
		return nil, grpcErr(err)
	}
	return &systemv1.GetOSInfoResponse{Os: info}, nil
}

func (s *Server) GetKernelInfo(context.Context, *systemv1.GetKernelInfoRequest) (*systemv1.GetKernelInfoResponse, error) {
	info, err := readKernelInfo()
	if err != nil {
		return nil, grpcErr(err)
	}
	return &systemv1.GetKernelInfoResponse{Kernel: info}, nil
}

func (s *Server) GetUptime(context.Context, *systemv1.GetUptimeRequest) (*systemv1.GetUptimeResponse, error) {
	uptime, err := readUptime()
	if err != nil {
		return nil, grpcErr(err)
	}
	return &systemv1.GetUptimeResponse{Uptime: uptime}, nil
}

func (s *Server) ListPackages(ctx context.Context, req *systemv1.ListPackagesRequest) (*systemv1.ListPackagesResponse, error) {
	manager := strings.TrimSpace(req.GetManager())
	if manager == "" {
		manager = detectPackageManager()
	}
	packages := listPackages(ctx, manager, reqLimit(int(req.GetLimit())))
	return &systemv1.ListPackagesResponse{Packages: packages}, nil
}

func (s *Server) ListServices(ctx context.Context, req *systemv1.ListServicesRequest) (*systemv1.ListServicesResponse, error) {
	services := listServices(ctx, firstNonBlank(req.GetManager(), "systemd"), req.GetState(), reqLimit(int(req.GetLimit())))
	return &systemv1.ListServicesResponse{Services: services}, nil
}

func (s *Server) ListUsers(_ context.Context, req *systemv1.ListUsersRequest) (*systemv1.ListUsersResponse, error) {
	users, err := readUsers(req.GetIncludeSystem())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &systemv1.ListUsersResponse{Users: users}, nil
}

func (s *Server) ListMounts(context.Context, *systemv1.ListMountsRequest) (*systemv1.ListMountsResponse, error) {
	mounts, err := readMounts()
	if err != nil {
		return nil, grpcErr(err)
	}
	return &systemv1.ListMountsResponse{Mounts: mounts}, nil
}

func (s *Server) GetDiskUsage(_ context.Context, req *systemv1.GetDiskUsageRequest) (*systemv1.GetDiskUsageResponse, error) {
	path := firstNonBlank(req.GetPath(), "/")
	usage, err := diskUsage(path)
	if err != nil {
		return nil, grpcErr(err)
	}
	return &systemv1.GetDiskUsageResponse{Disks: []*systemv1.DiskUsage{usage}}, nil
}

func (s *Server) ListProcesses(_ context.Context, req *systemv1.ListProcessesRequest) (*systemv1.ListProcessesResponse, error) {
	return &systemv1.ListProcessesResponse{Processes: listProcesses(reqLimit(int(req.GetLimit())), req.GetFilter())}, nil
}

func (s *Server) ListPorts(_ context.Context, req *systemv1.ListPortsRequest) (*systemv1.ListPortsResponse, error) {
	return &systemv1.ListPortsResponse{Ports: listPorts(req.GetProtocol(), reqLimit(int(req.GetLimit())))}, nil
}

func (s *Server) ListNetworkConnections(_ context.Context, req *systemv1.ListNetworkConnectionsRequest) (*systemv1.ListNetworkConnectionsResponse, error) {
	return &systemv1.ListNetworkConnectionsResponse{Connections: listConnections(req.GetState(), reqLimit(int(req.GetLimit())))}, nil
}

func (s *Server) ReadLogs(_ context.Context, req *systemv1.ReadLogsRequest) (*systemv1.ReadLogsResponse, error) {
	lines, err := readLogs(req.GetSource(), int(req.GetTailLines()), req.GetFilter())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &systemv1.ReadLogsResponse{Lines: lines}, nil
}

func (s *Server) GetMetrics(context.Context, *systemv1.GetMetricsRequest) (*systemv1.GetMetricsResponse, error) {
	return &systemv1.GetMetricsResponse{Metrics: readMetrics()}, nil
}

func (s *Server) KillProcess(_ context.Context, req *systemv1.KillProcessRequest) (*systemv1.KillProcessResponse, error) {
	if req.GetPid() <= 0 {
		return nil, serviceerrors.InvalidArgument("pid is required")
	}
	return &systemv1.KillProcessResponse{Plan: mutationPlan("system.kill_process", fmt.Sprint(req.GetPid()), req.GetReason(), "process.kill")}, nil
}

func (s *Server) RestartService(_ context.Context, req *systemv1.RestartServiceRequest) (*systemv1.RestartServiceResponse, error) {
	if strings.TrimSpace(req.GetName()) == "" {
		return nil, serviceerrors.InvalidArgument("name is required")
	}
	manager := firstNonBlank(req.GetManager(), "systemd")
	return &systemv1.RestartServiceResponse{Plan: mutationPlan("system.restart_service", manager+":"+req.GetName(), req.GetReason(), "service.restart")}, nil
}

func readOSInfo() (*systemv1.OSInfo, error) {
	values := keyValueFile("/etc/os-release")
	return &systemv1.OSInfo{
		Name:         firstNonBlank(values["NAME"], values["ID"]),
		Version:      values["VERSION_ID"],
		Id:           values["ID"],
		PrettyName:   firstNonBlank(values["PRETTY_NAME"], values["NAME"]),
		Architecture: runtime.GOARCH,
	}, nil
}

func readKernelInfo() (*systemv1.KernelInfo, error) {
	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err != nil {
		return nil, err
	}
	return &systemv1.KernelInfo{
		Name:    charsToString(uts.Sysname[:]),
		Release: charsToString(uts.Release[:]),
		Version: charsToString(uts.Version[:]),
		Machine: charsToString(uts.Machine[:]),
	}, nil
}

func readUptime() (*systemv1.Uptime, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return nil, fmt.Errorf("unexpected /proc/uptime format")
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, err
	}
	boot := time.Now().Add(-time.Duration(seconds) * time.Second)
	return &systemv1.Uptime{Seconds: seconds, BootTime: timestamppb.New(boot)}, nil
}

func readMounts() ([]*systemv1.Mount, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}
	mounts := make([]*systemv1.Mount, 0)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mounts = append(mounts, &systemv1.Mount{Device: fields[0], Path: fields[1], Filesystem: fields[2], Options: fields[3]})
	}
	return mounts, nil
}

func diskUsageForMounts(mounts []*systemv1.Mount) []*systemv1.DiskUsage {
	out := make([]*systemv1.DiskUsage, 0)
	seen := map[string]struct{}{}
	for _, mount := range mounts {
		if mount == nil || strings.HasPrefix(mount.GetPath(), "/proc") || strings.HasPrefix(mount.GetPath(), "/sys") {
			continue
		}
		if _, ok := seen[mount.GetPath()]; ok {
			continue
		}
		seen[mount.GetPath()] = struct{}{}
		if usage, err := diskUsage(mount.GetPath()); err == nil {
			out = append(out, usage)
		}
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func diskUsage(path string) (*systemv1.DiskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	return &systemv1.DiskUsage{Path: path, TotalBytes: total, FreeBytes: free, UsedBytes: total - free}, nil
}

func readMetrics() *systemv1.Metrics {
	metrics := &systemv1.Metrics{}
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			metrics.Load_1M, _ = strconv.ParseFloat(fields[0], 64)
			metrics.Load_5M, _ = strconv.ParseFloat(fields[1], 64)
			metrics.Load_15M, _ = strconv.ParseFloat(fields[2], 64)
		}
	}
	mem := keyValueUnits("/proc/meminfo")
	metrics.MemoryTotalBytes = mem["MemTotal"]
	metrics.MemoryAvailableBytes = mem["MemAvailable"]
	metrics.SwapTotalBytes = mem["SwapTotal"]
	metrics.SwapFreeBytes = mem["SwapFree"]
	return metrics
}

func readUsers(includeSystem bool) ([]*systemv1.User, error) {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil, err
	}
	users := make([]*systemv1.User, 0)
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		uid, _ := strconv.Atoi(parts[2])
		system := uid < 1000
		if system && !includeSystem {
			continue
		}
		users = append(users, &systemv1.User{Name: parts[0], Uid: parts[2], Gid: parts[3], Home: parts[5], Shell: parts[6], System: system})
	}
	return users, nil
}

func listProcesses(limit int, filter string) []*systemv1.Process {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]*systemv1.Process, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		proc := readProcess(pid)
		if proc == nil {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(proc.GetCommand()), filter) {
			continue
		}
		out = append(out, proc)
	}
	sortedProcesses(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func readProcess(pid int) *systemv1.Process {
	status := keyValueFile(filepath.Join("/proc", fmt.Sprint(pid), "status"))
	cmdline, _ := os.ReadFile(filepath.Join("/proc", fmt.Sprint(pid), "cmdline"))
	command := strings.ReplaceAll(string(cmdline), "\x00", " ")
	if strings.TrimSpace(command) == "" {
		command = status["Name"]
	}
	ppid, _ := strconv.ParseInt(status["PPid"], 10, 64)
	return &systemv1.Process{Pid: int64(pid), ParentPid: ppid, User: status["Uid"], Command: strings.TrimSpace(command), State: status["State"]}
}

func listPorts(protocol string, limit int) []*systemv1.Port {
	connections := listConnections("", limit*2)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	ports := make([]*systemv1.Port, 0)
	seen := map[string]struct{}{}
	for _, conn := range connections {
		if conn.GetState() != "LISTEN" && conn.GetProtocol() != "udp" && conn.GetProtocol() != "udp6" {
			continue
		}
		if protocol != "" && !strings.HasPrefix(conn.GetProtocol(), protocol) {
			continue
		}
		key := conn.GetProtocol() + "|" + conn.GetLocalAddress() + "|" + fmt.Sprint(conn.GetLocalPort())
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ports = append(ports, &systemv1.Port{Protocol: conn.GetProtocol(), Address: conn.GetLocalAddress(), Port: conn.GetLocalPort(), Pid: conn.GetPid()})
		if len(ports) >= limit {
			break
		}
	}
	return ports
}

func listConnections(state string, limit int) []*systemv1.NetworkConnection {
	files := []struct {
		path     string
		protocol string
	}{
		{"/proc/net/tcp", "tcp"},
		{"/proc/net/tcp6", "tcp6"},
		{"/proc/net/udp", "udp"},
		{"/proc/net/udp6", "udp6"},
	}
	state = strings.ToUpper(strings.TrimSpace(state))
	out := make([]*systemv1.NetworkConnection, 0)
	for _, file := range files {
		for _, conn := range parseNetFile(file.path, file.protocol) {
			if state != "" && conn.GetState() != state {
				continue
			}
			out = append(out, conn)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func parseNetFile(path, protocol string) []*systemv1.NetworkConnection {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := make([]*systemv1.NetworkConnection, 0)
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		localAddr, localPort := parseEndpoint(fields[1])
		remoteAddr, remotePort := parseEndpoint(fields[2])
		out = append(out, &systemv1.NetworkConnection{
			Protocol:      protocol,
			LocalAddress:  localAddr,
			LocalPort:     localPort,
			RemoteAddress: remoteAddr,
			RemotePort:    remotePort,
			State:         tcpState(fields[3]),
		})
	}
	return out
}

func parseEndpoint(raw string) (string, uint32) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return "", 0
	}
	port64, _ := strconv.ParseUint(parts[1], 16, 32)
	addr := parts[0]
	if len(addr) == 8 {
		bytes := make([]byte, 4)
		for i := 0; i < 4; i++ {
			v, _ := strconv.ParseUint(addr[i*2:i*2+2], 16, 8)
			bytes[3-i] = byte(v)
		}
		return net.IP(bytes).String(), uint32(port64)
	}
	return addr, uint32(port64)
}

func tcpState(code string) string {
	switch code {
	case "0A":
		return "LISTEN"
	case "01":
		return "ESTABLISHED"
	case "06":
		return "TIME_WAIT"
	case "07":
		return "CLOSE"
	default:
		return code
	}
}

func readLogs(source string, tailLines int, filter string) ([]*systemv1.LogLine, error) {
	path, err := logPath(source)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if tailLines <= 0 || tailLines > 200 {
		tailLines = 100
	}
	if len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]*systemv1.LogLine, 0, len(lines))
	for _, line := range lines {
		if filter != "" && !strings.Contains(strings.ToLower(line), filter) {
			continue
		}
		out = append(out, &systemv1.LogLine{Source: path, Message: line})
	}
	return out, nil
}

func logPath(source string) (string, error) {
	source = strings.TrimSpace(source)
	switch source {
	case "", "syslog":
		source = "/var/log/syslog"
	case "messages":
		source = "/var/log/messages"
	case "kern":
		source = "/var/log/kern.log"
	}
	if !filepath.IsAbs(source) {
		return "", fmt.Errorf("log source must be an approved absolute /var/log path")
	}
	clean := filepath.Clean(source)
	if clean != "/var/log" && !strings.HasPrefix(clean, "/var/log/") {
		return "", fmt.Errorf("log source must be under /var/log")
	}
	return clean, nil
}

func listPackages(ctx context.Context, manager string, limit int) []*systemv1.Package {
	switch manager {
	case "dpkg":
		return dpkgPackages(ctx, limit)
	case "rpm":
		return rpmPackages(ctx, limit)
	default:
		return nil
	}
}

func detectPackageManager() string {
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		return "dpkg"
	}
	if _, err := exec.LookPath("rpm"); err == nil {
		return "rpm"
	}
	return ""
}

func dpkgPackages(ctx context.Context, limit int) []*systemv1.Package {
	out, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${Package}\t${Version}\t${Architecture}\n").Output()
	if err != nil {
		return nil
	}
	return packageLines(string(out), "dpkg", limit)
}

func rpmPackages(ctx context.Context, limit int) []*systemv1.Package {
	out, err := exec.CommandContext(ctx, "rpm", "-qa", "--qf", "%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\n").Output()
	if err != nil {
		return nil
	}
	return packageLines(string(out), "rpm", limit)
}

func packageLines(value, manager string, limit int) []*systemv1.Package {
	packages := make([]*systemv1.Package, 0)
	for _, line := range strings.Split(value, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		packages = append(packages, &systemv1.Package{Name: parts[0], Version: parts[1], Architecture: parts[2], Manager: manager})
		if len(packages) >= limit {
			break
		}
	}
	return packages
}

func listServices(ctx context.Context, manager, state string, limit int) []*systemv1.Service {
	if manager != "systemd" {
		return nil
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil
	}
	args := []string{"list-units", "--type=service", "--no-pager", "--plain", "--all"}
	out, err := exec.CommandContext(ctx, "systemctl", args...).Output()
	if err != nil {
		return nil
	}
	services := make([]*systemv1.Service, 0)
	state = strings.ToLower(strings.TrimSpace(state))
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		active := fields[2]
		if state != "" && active != state {
			continue
		}
		description := ""
		if len(fields) > 4 {
			description = strings.Join(fields[4:], " ")
		}
		services = append(services, &systemv1.Service{Name: fields[0], State: active, Manager: "systemd", Description: description})
		if len(services) >= limit {
			break
		}
	}
	return services
}

func mutationPlan(action, target, reason, risk string) *systemv1.MutationPlan {
	seed := action + "|" + target + "|" + reason + "|" + risk
	sum := sha256.Sum256([]byte(seed))
	return &systemv1.MutationPlan{
		Id:               hex.EncodeToString(sum[:8]),
		Action:           action,
		Target:           target,
		Reason:           strings.TrimSpace(reason),
		ApprovalRequired: true,
		Risks:            []string{risk},
	}
}

func keyValueFile(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
			if !ok {
				continue
			}
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return out
}

func keyValueUnits(path string) map[string]uint64 {
	data := keyValueFile(path)
	out := map[string]uint64{}
	for key, raw := range data {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		value, _ := strconv.ParseUint(fields[0], 10, 64)
		if len(fields) > 1 && strings.EqualFold(fields[1], "kb") {
			value *= 1024
		}
		out[key] = value
	}
	return out
}

func charsToString(chars []int8) string {
	b := make([]byte, 0, len(chars))
	for _, ch := range chars {
		if ch == 0 {
			break
		}
		b = append(b, byte(ch))
	}
	return string(b)
}

func reqLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func grpcErr(err error) error {
	if err == nil {
		return nil
	}
	return serviceerrors.InvalidArgument(err.Error())
}

func sortedProcesses(processes []*systemv1.Process) {
	sort.Slice(processes, func(i, j int) bool { return processes[i].GetPid() < processes[j].GetPid() })
}
