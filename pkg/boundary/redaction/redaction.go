// Package redaction provides deterministic boundary sanitization for secrets
// before data is stored, logged, or written as test artifacts.
package redaction

import (
	"os"
	"regexp"
	"strings"
)

const placeholder = "[redacted]"

var sensitiveKeyMarkers = []string{
	"api_key",
	"apikey",
	"authorization",
	"client_secret",
	"credential",
	"jwt",
	"oauth",
	"password",
	"private_key",
	"refresh_token",
	"secret",
	"token",
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization["':=\s]+(?:bearer|basic)\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|client[_-]?secret|password|refresh[_-]?token|token|secret)["':=\s]+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)(sk-(?:or-v1-)?)[A-Za-z0-9._~+/=-]+`),
}

// RedactString returns value with known secret values and common credential
// patterns replaced by a stable placeholder.
func RedactString(value string) string {
	redacted := value
	for _, env := range os.Environ() {
		key, secret, ok := strings.Cut(env, "=")
		if !ok || len(secret) < 6 || !LooksSensitiveKey(key) {
			continue
		}
		redacted = strings.ReplaceAll(redacted, secret, placeholder)
	}
	for _, pattern := range secretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "${1}"+placeholder)
	}
	return redacted
}

// RedactBytes returns an independent byte slice with RedactString applied.
func RedactBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return []byte(RedactString(string(value)))
}

// LooksSensitiveKey reports whether key is a credential-bearing environment or
// structured payload key.
func LooksSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	for _, marker := range sensitiveKeyMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
