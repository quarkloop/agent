//go:build e2e

package utils

import "testing"

// Logf writes one harness log line with an unambiguous E2E owner prefix.
func Logf(t testing.TB, format string, args ...any) {
	t.Helper()
	allArgs := append([]any{t.Name()}, args...)
	t.Logf("[e2e][test=%s] "+format, allArgs...)
}
