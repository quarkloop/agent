// Package sourceid provides canonical comparison for runtime source locators.
package sourceid

import (
	"net/url"
	"path/filepath"
	"strings"
)

// Canonical returns a stable identity for local filesystem source locators.
// Non-file URIs are retained because their scheme and authority are semantic.
func Canonical(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err == nil && strings.EqualFold(parsed.Scheme, "file") && (parsed.Host == "" || strings.EqualFold(parsed.Host, "localhost")) {
		if parsed.Path != "" {
			return filepath.Clean(parsed.Path)
		}
	}
	if !strings.Contains(value, "://") && filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return value
}

// Equal reports whether two source locators identify the same canonical source.
func Equal(left, right string) bool {
	return Canonical(left) == Canonical(right)
}
