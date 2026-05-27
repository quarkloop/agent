//go:build e2e

package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

const natsCLITimeout = 750 * time.Millisecond

type natsCLIIdentity struct {
	User     string
	Password string
}

type natsCLIProbe struct {
	Label    string
	Identity natsCLIIdentity
	Args     []string
}

func DumpNATSCLIDiagnostics(t testing.TB, endpoints NATSEndpoints, label string, services []string, artifactDirs ...string) {
	t.Helper()
	binary, ok := resolveNATSCLI()
	if !ok {
		Logf(t, "nats-cli[%s] unavailable; install nats or set NATS_CLI to enable subject and account diagnostics", label)
		writeNATSCLIArtifact(t, artifactDirs, label, "unavailable", "nats CLI unavailable")
		return
	}
	if endpoints.ClientURL == "" {
		Logf(t, "nats-cli[%s] skipped: empty NATS client URL", label)
		writeNATSCLIArtifact(t, artifactDirs, label, "skipped", "empty NATS client URL")
		return
	}
	control := natsCLIIdentity{User: natshub.DefaultControlUser, Password: natshub.DefaultControlPassword}
	system := natsCLIIdentity{User: natshub.DefaultSystemUser, Password: natshub.DefaultSystemPassword}
	probes := []natsCLIProbe{
		{Label: "account-info", Identity: control, Args: []string{"account", "info"}},
		{Label: "streams", Identity: control, Args: []string{"stream", "list", "--names", "--all"}},
		{Label: "kv", Identity: control, Args: []string{"kv", "list", "--names"}},
		{Label: "services-api", Identity: control, Args: []string{"service", "list", "--json"}},
		{Label: "accounts", Identity: system, Args: []string{"server", "report", "accounts", "--json"}},
		{Label: "session-subscriptions", Identity: system, Args: []string{"server", "request", "subscriptions", "--filter-subject", "session.e2e.input", "--detail", "1"}},
	}
	probes = append(probes, serviceSubscriptionProbes(system, services)...)
	for _, probe := range probes {
		out, err := runNATSCLIProbe(binary, endpoints.ClientURL, probe)
		status := "ok"
		if err != nil {
			status = "error: " + err.Error()
		}
		Logf(t, "nats-cli[%s][%s] status=%s output=%s", label, probe.Label, status, compactCLIOutput(out))
		writeNATSCLIArtifact(t, artifactDirs, label, probe.Label, fmt.Sprintf("status=%s\n%s", status, out))
	}
}

func serviceSubscriptionProbes(identity natsCLIIdentity, services []string) []natsCLIProbe {
	probes := make([]natsCLIProbe, 0, len(services)+3)
	for _, service := range uniqueNonEmpty(services) {
		function, ok := serviceReadinessFunctions[service]
		if !ok {
			continue
		}
		operation, err := natskit.ServiceOperationFromFunctionName(service, function)
		if err != nil {
			continue
		}
		probes = append(probes, subscriptionProbe("svc-"+service+"-subscriptions", identity, operation.Subject))
	}
	if containsString(services, "gateway") {
		probes = append(probes,
			subscriptionProbe("svc-gateway-stream-subscriptions", identity, "svc.gateway.v1.stream_generate"),
			subscriptionProbe("svc-gateway-model-catalog-subscriptions", identity, "svc.gateway.v1.list_models"),
			subscriptionProbe("svc-gateway-embedding-subscriptions", identity, "svc.gateway.v1.embed"),
		)
	}
	return probes
}

func subscriptionProbe(label string, identity natsCLIIdentity, subject string) natsCLIProbe {
	return natsCLIProbe{Label: label, Identity: identity, Args: []string{"server", "request", "subscriptions", "--filter-subject", subject, "--detail", "1"}}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func writeNATSCLIArtifact(t testing.TB, dirs []string, label, probe, content string) {
	t.Helper()
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			Logf(t, "create NATS diagnostic artifact directory %s: %v", dir, err)
			continue
		}
		path := filepath.Join(dir, "nats-"+label+"-"+probe+".txt")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			Logf(t, "write NATS diagnostic artifact %s: %v", path, err)
			continue
		}
		Logf(t, "nats diagnostic artifact: %s", path)
	}
}

func resolveNATSCLI() (string, bool) {
	if configured := strings.TrimSpace(os.Getenv("NATS_CLI")); configured != "" {
		return configured, true
	}
	if path, err := exec.LookPath("nats"); err == nil {
		return path, true
	}
	return "", false
}

func runNATSCLIProbe(binary, url string, probe natsCLIProbe) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{
		"--server", url,
		"--user", probe.Identity.User,
		"--password", probe.Identity.Password,
		"--timeout", natsCLITimeout.String(),
		"--no-context",
	}
	args = append(args, probe.Args...)
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(out), ctx.Err()
	}
	return string(out), err
}

func compactCLIOutput(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return "<empty>"
	}
	out = strings.Join(strings.Fields(out), " ")
	const maxLen = 2000
	if len(out) <= maxLen {
		return out
	}
	return out[:maxLen] + "...<truncated>"
}
