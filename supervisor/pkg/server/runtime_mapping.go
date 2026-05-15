package server

import (
	"time"

	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/runtime"
)

func toAPIRuntimeInfo(a *runtime.Runtime) api.RuntimeInfo {
	info := api.RuntimeInfo{
		ID:         a.ID(),
		Space:      a.Space(),
		WorkingDir: a.WorkingDir(),
		Status:     a.Status(),
		PID:        a.PID(),
		Port:       a.Port(),
		StartedAt:  a.StartedAt(),
	}
	if a.Status() == api.RuntimeRunning && !a.StartedAt().IsZero() {
		info.Uptime = time.Since(a.StartedAt()).Round(time.Second).String()
	}
	return info
}
