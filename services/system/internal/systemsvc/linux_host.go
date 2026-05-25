package systemsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type nativeHost struct{}

func (nativeHost) OSInfo() (*systemv1.OSInfo, error) {
	values := keyValueFile("/etc/os-release")
	return &systemv1.OSInfo{
		Name:         firstNonBlank(values["NAME"], values["ID"]),
		Version:      values["VERSION_ID"],
		Id:           values["ID"],
		PrettyName:   firstNonBlank(values["PRETTY_NAME"], values["NAME"]),
		Architecture: runtime.GOARCH,
	}, nil
}

func (nativeHost) KernelInfo() (*systemv1.KernelInfo, error) {
	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err != nil {
		return nil, err
	}
	return &systemv1.KernelInfo{
		Name: charsToString(uts.Sysname[:]), Release: charsToString(uts.Release[:]),
		Version: charsToString(uts.Version[:]), Machine: charsToString(uts.Machine[:]),
	}, nil
}

func (nativeHost) Uptime() (*systemv1.Uptime, error) {
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
	return &systemv1.Uptime{Seconds: seconds, BootTime: timestamppb.New(time.Now().Add(-time.Duration(seconds) * time.Second))}, nil
}

func (nativeHost) Mounts() ([]*systemv1.Mount, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}
	mounts := make([]*systemv1.Mount, 0)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			mounts = append(mounts, &systemv1.Mount{Device: fields[0], Path: fields[1], Filesystem: fields[2], Options: fields[3]})
		}
	}
	return mounts, nil
}

func (nativeHost) DiskUsageForMounts(mounts []*systemv1.Mount) []*systemv1.DiskUsage {
	out := make([]*systemv1.DiskUsage, 0)
	seen := map[string]struct{}{}
	host := nativeHost{}
	for _, mount := range mounts {
		if mount == nil || strings.HasPrefix(mount.GetPath(), "/proc") || strings.HasPrefix(mount.GetPath(), "/sys") {
			continue
		}
		if _, ok := seen[mount.GetPath()]; ok {
			continue
		}
		seen[mount.GetPath()] = struct{}{}
		if usage, err := host.DiskUsage(mount.GetPath()); err == nil {
			out = append(out, usage)
		}
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func (nativeHost) DiskUsage(path string) (*systemv1.DiskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	return &systemv1.DiskUsage{Path: path, TotalBytes: total, FreeBytes: free, UsedBytes: total - free}, nil
}

func (nativeHost) Metrics() *systemv1.Metrics {
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

func (nativeHost) Users(includeSystem bool) ([]*systemv1.User, error) {
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
		if !system || includeSystem {
			users = append(users, &systemv1.User{Name: parts[0], Uid: parts[2], Gid: parts[3], Home: parts[5], Shell: parts[6], System: system})
		}
	}
	return users, nil
}

func (nativeHost) Processes(limit int, filter string) ([]*systemv1.Process, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]*systemv1.Process, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		proc := readProcess(pid)
		if proc != nil && (filter == "" || strings.Contains(strings.ToLower(proc.GetCommand()), filter)) {
			out = append(out, proc)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetPid() < out[j].GetPid() })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
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
