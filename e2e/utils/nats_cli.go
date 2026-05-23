//go:build e2e

package utils

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

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

func DumpNATSCLIDiagnostics(t testing.TB, endpoints NATSEndpoints, label string) {
	t.Helper()
	binary, ok := resolveNATSCLI()
	if !ok {
		Logf(t, "nats-cli[%s] unavailable; install nats or set NATS_CLI to enable subject and account diagnostics", label)
		return
	}
	if endpoints.ClientURL == "" {
		Logf(t, "nats-cli[%s] skipped: empty NATS client URL", label)
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
		{Label: "svc-io-subscriptions", Identity: system, Args: []string{"server", "request", "subscriptions", "--filter-subject", "svc.io.v1.read", "--detail", "1"}},
		{Label: "svc-gateway-subscriptions", Identity: system, Args: []string{"server", "request", "subscriptions", "--filter-subject", "svc.gateway.v1.stream_generate", "--detail", "1"}},
		{Label: "svc-embedding-subscriptions", Identity: system, Args: []string{"server", "request", "subscriptions", "--filter-subject", "svc.embedding.v1.embed", "--detail", "1"}},
		{Label: "svc-indexer-subscriptions", Identity: system, Args: []string{"server", "request", "subscriptions", "--filter-subject", "svc.indexer.v1.query_context", "--detail", "1"}},
		{Label: "session-subscriptions", Identity: system, Args: []string{"server", "request", "subscriptions", "--filter-subject", "session.e2e.input", "--detail", "1"}},
	}
	for _, probe := range probes {
		out, err := runNATSCLIProbe(binary, endpoints.ClientURL, probe)
		status := "ok"
		if err != nil {
			status = "error: " + err.Error()
		}
		Logf(t, "nats-cli[%s][%s] status=%s output=%s", label, probe.Label, status, compactCLIOutput(out))
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
