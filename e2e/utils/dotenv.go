//go:build e2e

package utils

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	// utils/dotenv.go → e2e/ → quark/ → workspace/
	quarkRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	workspaceRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	loadDotEnv(filepath.Join(quarkRoot, ".env"))
	loadDotEnv(filepath.Join(workspaceRoot, ".env"))
}

// ProviderConfig identifies which real LLM provider drives E2E execution.
type ProviderConfig struct {
	Provider string
	Model    string
	APIKey   string
}

const defaultE2EModel = "openrouter/owl-alpha"

var allowedE2EModels = map[string]struct{}{
	"openrouter/owl-alpha":                   {},
	"nvidia/nemotron-3-super-120b-a12b:free": {},
	"deepseek/deepseek-v4-flash:free":        {},
}

// RequireProviderConfig returns the real Gateway provider configuration for
// E2E execution. A missing provider is a failed prerequisite, not a silently
// skipped production-flow test.
func RequireProviderConfig(t *testing.T) ProviderConfig {
	t.Helper()
	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		m := firstEnv("OPENROUTER_E2E_MODEL", "OPENROUTER_MODEL")
		if m == "" {
			m = defaultE2EModel
		}
		if !allowedE2EModel(m) {
			t.Fatalf("OpenRouter E2E model %q is not allowed; configure one of the declared real-model E2E allowlist values", m)
		}
		return ProviderConfig{Provider: "openrouter", Model: m, APIKey: key}
	}
	t.Fatal("OPENROUTER_API_KEY is required for real-model E2E execution")
	return ProviderConfig{}
}

func allowedE2EModel(model string) bool {
	_, ok := allowedE2EModels[strings.TrimSpace(model)]
	return ok
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
