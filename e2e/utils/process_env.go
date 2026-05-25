//go:build e2e

package utils

import (
	"os"
	"sort"
	"strings"
)

func SupervisorProcessEnv(overrides map[string]string) []string {
	return constrainedProcessEnv(overrides, providerCredentialEnvKeys()...)
}

// ServiceProcessEnv returns a constrained service process environment. Service
// start helpers must pass provider keys explicitly when that service requires
// provider access.
func ServiceProcessEnv(overrides map[string]string, extraAllowed ...string) []string {
	values := map[string]string{
		"QUARK_NATS_AUDIT_PREFIX":     "audit",
		"QUARK_NATS_TELEMETRY_PREFIX": "telemetry",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return constrainedProcessEnv(values, extraAllowed...)
}

// RuntimeProcessEnv returns the constrained environment needed by a runtime
// launched directly by the e2e harness. Runtime launch is deployment-owned in
// the NATS-native architecture; the supervisor no longer spawns the process.
func RuntimeProcessEnv(overrides map[string]string) []string {
	return constrainedProcessEnv(overrides, providerCredentialEnvKeys()...)
}

func constrainedProcessEnv(overrides map[string]string, extraAllowed ...string) []string {
	allowed := map[string]struct{}{}
	for _, key := range baseProcessEnvKeys() {
		allowed[key] = struct{}{}
	}
	for _, key := range extraAllowed {
		if key != "" {
			allowed[key] = struct{}{}
		}
	}
	values := make(map[string]string, len(allowed)+len(overrides))
	order := make([]string, 0, len(allowed)+len(overrides))
	add := func(key, value string) {
		if key == "" {
			return
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, keep := allowed[key]; keep {
			add(key, value)
		}
	}
	overrideKeys := make([]string, 0, len(overrides))
	for key := range overrides {
		overrideKeys = append(overrideKeys, key)
	}
	sort.Strings(overrideKeys)
	for _, key := range overrideKeys {
		add(key, overrides[key])
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, key+"="+values[key])
	}
	return out
}

func baseProcessEnvKeys() []string {
	return []string{
		"PATH",
		"HOME",
		"USER",
		"LOGNAME",
		"TMPDIR",
		"TEMP",
		"TMP",
		"LANG",
		"LC_ALL",
		"SSL_CERT_FILE",
		"SSL_CERT_DIR",
		"TZ",
		"TERM",
		"XDG_CACHE_HOME",
		"XDG_CONFIG_HOME",
		"XDG_DATA_HOME",
		"GOCACHE",
		"GOMODCACHE",
		"GOPATH",
		"GOROOT",
		"GOENV",
		"GOFLAGS",
		"CGO_ENABLED",
		"DOCKER_CONFIG",
		"DOCKER_CONTEXT",
		"DOCKER_HOST",
	}
}

func providerCredentialEnvKeys() []string {
	return []string{
		"OPENROUTER_API_KEY",
		"OPENROUTER_BASE_URL",
		"OPENAI_API_KEY",
		"OPENAI_BASE_URL",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_BASE_URL",
		"ZHIPU_API_KEY",
		"ZHIPU_BASE_URL",
	}
}
