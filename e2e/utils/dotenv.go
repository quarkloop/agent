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

// IsRateLimit reports whether err looks like an upstream rate-limit response.
func IsRateLimit(err error) bool {
	return IsRateLimitText(err.Error())
}

func IsRateLimitText(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "402") ||
		strings.Contains(msg, "quota") ||
		strings.Contains(msg, "credit") ||
		strings.Contains(msg, "requires more credits") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "rate-limit") ||
		strings.Contains(msg, "rate-limited")
}

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

// ProviderConfig identifies which LLM provider to drive an e2e test against.
type ProviderConfig struct {
	Provider string
	Model    string
	APIKey   string
}

// CfgForTest returns the provider configuration derived from env vars. The
// second return is false when no credentials are present, so the caller can
// skip.
func CfgForTest(t *testing.T, envKey string) (ProviderConfig, bool) {
	t.Helper()
	if key := os.Getenv(envKey); key != "" {
		m := firstEnv("OPENROUTER_E2E_MODEL", "OPENROUTER_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL")
		if m == "" {
			m = "openai/gpt-4o-mini"
		}
		return ProviderConfig{Provider: "openrouter", Model: m, APIKey: key}, true
	}
	return ProviderConfig{}, false
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
