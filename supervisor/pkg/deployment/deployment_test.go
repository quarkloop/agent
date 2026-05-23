package deployment

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComposeDeclaresOperatorManagedProcesses(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "compose", "quark.yml"))
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	var compose struct {
		Services map[string]struct {
			Command     []string          `yaml:"command"`
			DependsOn   map[string]any    `yaml:"depends_on"`
			Healthcheck map[string]any    `yaml:"healthcheck"`
			Profiles    []string          `yaml:"profiles"`
			Build       map[string]any    `yaml:"build"`
			Environment map[string]string `yaml:"environment"`
			Restart     string            `yaml:"restart"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatalf("parse compose: %v", err)
	}
	for _, name := range []string{"supervisor", "runtime", "io", "space", "indexer", "embedding", "vector", "nats-exporter", "vmagent", "victoria-metrics", "openbao", "temporal"} {
		if _, ok := compose.Services[name]; !ok {
			t.Fatalf("compose service %q missing", name)
		}
	}
	supervisor := compose.Services["supervisor"]
	if contains(supervisor.Command, "--runtime") {
		t.Fatalf("supervisor command still accepts runtime launch ownership: %v", supervisor.Command)
	}
	if len(supervisor.Healthcheck) == 0 {
		t.Fatal("supervisor healthcheck missing")
	}
	runtimeSvc := compose.Services["runtime"]
	if _, ok := runtimeSvc.DependsOn["supervisor"]; !ok {
		t.Fatal("runtime does not depend on supervisor health")
	}
	if runtimeSvc.Environment["QUARK_SUPERVISOR_URL"] == "" || runtimeSvc.Environment["QUARK_SPACE"] == "" {
		t.Fatalf("runtime environment incomplete: %+v", runtimeSvc.Environment)
	}
	for name, svc := range compose.Services {
		if name == "dgraph" || name == "vector" || name == "nats-exporter" || name == "vmagent" || name == "victoria-metrics" || name == "openbao" || name == "temporal" {
			continue
		}
		if len(svc.Build) == 0 {
			t.Fatalf("service %q does not declare a local build", name)
		}
		if svc.Restart == "" {
			t.Fatalf("service %q does not declare a restart policy", name)
		}
	}
}

func TestObservabilityDeploymentDeclaresNATSTelemetryAndMetrics(t *testing.T) {
	root := repoRoot(t)
	vectorData, err := os.ReadFile(filepath.Join(root, "deploy", "vector", "vector.toml"))
	if err != nil {
		t.Fatalf("read vector config: %v", err)
	}
	vectorText := string(vectorData)
	for _, want := range []string{"type = \"nats\"", "subject = \"audit.>\"", "subject = \"telemetry.>\"", "strategy = \"user_password\""} {
		if !strings.Contains(vectorText, want) {
			t.Fatalf("vector config missing %q", want)
		}
	}

	vmagentData, err := os.ReadFile(filepath.Join(root, "deploy", "victoria", "vmagent.yml"))
	if err != nil {
		t.Fatalf("read vmagent config: %v", err)
	}
	vmagentText := string(vmagentData)
	for _, want := range []string{"job_name: quark_nats", "nats-exporter:7777", "job_name: quark_vmagent"} {
		if !strings.Contains(vmagentText, want) {
			t.Fatalf("vmagent config missing %q", want)
		}
	}
}

func TestDockerfileBuildsSingleGoPackage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "deploy", "docker", "Dockerfile.go-binary"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	text := string(data)
	for _, want := range []string{"ARG PACKAGE", "go build", "ENTRYPOINT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Dockerfile missing %q", want)
		}
	}
}

func TestSystemdTemplatesKeepLifecycleOutsideSupervisor(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		"deploy/systemd/quark-supervisor.service",
		"deploy/systemd/quark-runtime@.service",
		"deploy/systemd/quark-service@.service",
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(data)
		if !strings.Contains(text, "Restart=on-failure") {
			t.Fatalf("%s missing restart policy", file)
		}
		if strings.Contains(text, "--runtime") {
			t.Fatalf("%s still wires supervisor runtime launch", file)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
