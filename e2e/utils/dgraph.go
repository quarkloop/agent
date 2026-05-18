//go:build e2e

package utils

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
)

const DgraphStandaloneImage = "dgraph/standalone:v25.0.0"

func StartDgraph(t *testing.T) string {
	t.Helper()
	port := ReservePort(t)
	name := fmt.Sprintf("quark-e2e-dgraph-%d", time.Now().UnixNano())
	StartProcess(t, "dgraph", "docker", []string{
		"run",
		"--rm",
		"--name", name,
		"-p", fmt.Sprintf("127.0.0.1:%d:9080", port),
		DgraphStandaloneImage,
	}, ServiceProcessEnv(nil))
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	WaitForTCP(t, addr, 90*time.Second)
	return addr
}
