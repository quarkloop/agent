//go:build e2e

package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

var composeTokenCleaner = regexp.MustCompile(`[^a-z0-9]+`)

// ComposeProject owns the isolated product deployment for one E2E scenario.
// Product binaries are launched only through Docker Compose.
type ComposeProject struct {
	t         *testing.T
	root      string
	file      string
	name      string
	env       map[string]string
	artifacts string
	endpoints NATSEndpoints
}

func NewComposeProject(t *testing.T, workingDir string) *ComposeProject {
	t.Helper()
	root := QuarkRoot(t)
	name := composeProjectName(t.Name())
	artifacts := filepath.Join(t.TempDir(), "compose-artifacts")
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		t.Fatalf("mkdir compose artifacts: %v", err)
	}
	clientPort := ReservePort(t)
	monitorPort := ReservePort(t)
	webSocketPort := ReservePort(t)
	project := &ComposeProject{
		t:         t,
		root:      root,
		file:      filepath.Join(root, "deploy", "compose", "quark.yml"),
		name:      name,
		artifacts: artifacts,
		env: map[string]string{
			"QUARK_NATS_CLIENT_PORT":    fmt.Sprint(clientPort),
			"QUARK_NATS_MONITOR_PORT":   fmt.Sprint(monitorPort),
			"QUARK_NATS_WEBSOCKET_PORT": fmt.Sprint(webSocketPort),
			"QUARK_E2E_WORKING_DIR":     workingDir,
			// E2E fixtures deliberately exercise approved writes in the
			// isolated bind-mounted workspace.
			"QUARK_CONTAINER_USER": "0:0",
		},
		endpoints: NATSEndpoints{
			ClientURL:     fmt.Sprintf("nats://127.0.0.1:%d", clientPort),
			MonitoringURL: fmt.Sprintf("http://127.0.0.1:%d", monitorPort),
			WebSocketURL:  fmt.Sprintf("ws://127.0.0.1:%d", webSocketPort),
		},
	}
	t.Cleanup(func() {
		project.capture("cleanup")
		project.run(false, "down", "--volumes", "--remove-orphans")
	})
	return project
}

func composeProjectName(testName string) string {
	base := composeTokenCleaner.ReplaceAllString(strings.ToLower(testName), "")
	if base == "" {
		base = "scenario"
	}
	if len(base) > 28 {
		base = base[:28]
	}
	return fmt.Sprintf("quarke2e%s%d", base, time.Now().UnixNano())
}

func (p *ComposeProject) Endpoints() NATSEndpoints { return p.endpoints }

func (p *ComposeProject) ArtifactsDir() string { return p.artifacts }

func (p *ComposeProject) SetEnv(values map[string]string) {
	for key, value := range values {
		if strings.TrimSpace(key) != "" {
			p.env[key] = value
		}
	}
}

func (p *ComposeProject) Up(services ...string) {
	p.t.Helper()
	services = uniqueNonEmpty(services)
	args := append([]string{"up", "--build", "--detach"}, services...)
	p.require(args...)
	Logf(p.t, "docker-compose project=%s services=%s nats=%s", p.name, strings.Join(services, ","), p.endpoints.ClientURL)
}

// RunServiceExpectFailure runs an additional Compose-owned service instance
// and returns its diagnostics after the process rejects the requested startup.
func (p *ComposeProject) RunServiceExpectFailure(service string, overrides map[string]string) string {
	p.t.Helper()
	args := []string{"run", "--no-deps", "--rm"}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", key+"="+overrides[key])
	}
	args = append(args, service)
	output, err := p.run(true, args...)
	if err == nil {
		p.t.Fatalf("docker compose run %s unexpectedly succeeded\n%s", service, output)
	}
	Logf(p.t, "expected compose rejection service=%s output=%s", service, compactCLIOutput(output))
	return output
}

func (p *ComposeProject) capture(label string) {
	p.t.Helper()
	for _, capture := range []struct {
		name string
		args []string
	}{
		{name: "ps", args: []string{"ps", "--all", "--format", "json"}},
		{name: "logs", args: []string{"logs", "--no-color", "--timestamps"}},
		// Never write expanded Compose environment values to artifacts:
		// provider keys are passed to Compose for Gateway startup.
		{name: "config-services", args: []string{"config", "--services"}},
	} {
		output, _ := p.run(true, capture.args...)
		path := filepath.Join(p.artifacts, label+"-"+capture.name+".txt")
		if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
			Logf(p.t, "write compose artifact %s: %v", path, err)
			continue
		}
		Logf(p.t, "compose artifact: %s", path)
	}
}

func (p *ComposeProject) require(args ...string) string {
	p.t.Helper()
	output, err := p.run(true, args...)
	if err != nil {
		p.capture("failure")
		tail := output
		if len(tail) > 4000 {
			tail = tail[len(tail)-4000:]
		}
		p.t.Fatalf("docker compose %s: %v\n%s", strings.Join(args, " "), err, tail)
	}
	return output
}

func (p *ComposeProject) run(includeProfiles bool, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	base := []string{"compose", "--project-name", p.name, "--file", p.file}
	if includeProfiles {
		for _, profile := range []string{"runtime", "services", "knowledge", "gateway", "devops", "system", "workflow", "secrets", "observability"} {
			base = append(base, "--profile", profile)
		}
	}
	cmd := exec.CommandContext(ctx, "docker", append(base, args...)...)
	cmd.Dir = p.root
	cmd.Env = p.environment()
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), ctx.Err()
	}
	return string(output), err
}

func (p *ComposeProject) environment() []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range p.env {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
