package systemsvc

import (
	"context"

	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
)

type hostReader interface {
	OSInfo() (*systemv1.OSInfo, error)
	KernelInfo() (*systemv1.KernelInfo, error)
	Uptime() (*systemv1.Uptime, error)
	Mounts() ([]*systemv1.Mount, error)
	DiskUsage(string) (*systemv1.DiskUsage, error)
	DiskUsageForMounts([]*systemv1.Mount) []*systemv1.DiskUsage
	Users(bool) ([]*systemv1.User, error)
	Processes(int, string) ([]*systemv1.Process, error)
	Metrics() *systemv1.Metrics
}

type networkReader interface {
	Ports(string, int) ([]*systemv1.Port, error)
	Connections(string, int) ([]*systemv1.NetworkConnection, error)
}

type inventoryReader interface {
	Packages(context.Context, string, int) ([]*systemv1.Package, error)
	Services(context.Context, string, string, int) ([]*systemv1.Service, error)
	DefaultPackageManager() string
}

type logReader interface {
	Read(string, int, string) ([]*systemv1.LogLine, error)
}

type mutationPlanner interface {
	KillProcess(int64, string) *systemv1.MutationPlan
	RestartService(string, string, string) *systemv1.MutationPlan
}

type Server struct {
	host      hostReader
	network   networkReader
	inventory inventoryReader
	logs      logReader
	planner   mutationPlanner
}

func NewServer() *Server {
	return newServer(nativeHost{}, procNetwork{}, commandInventory{}, approvedLogs{}, approvalPlanner{})
}

func newServer(host hostReader, network networkReader, inventory inventoryReader, logs logReader, planner mutationPlanner) *Server {
	return &Server{host: host, network: network, inventory: inventory, logs: logs, planner: planner}
}
