//go:build e2e

package e2e_test

import "strings"

func isProviderRateLimited(message string) bool {
	normalized := strings.ToLower(message)
	return strings.Contains(normalized, "rate_limit") ||
		strings.Contains(normalized, "rate limited") ||
		strings.Contains(normalized, "rate limit") ||
		strings.Contains(normalized, "429")
}
