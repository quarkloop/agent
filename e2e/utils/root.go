//go:build e2e

package utils

import (
	"path/filepath"
	"runtime"
	"testing"
)

// QuarkRoot returns the absolute path to the repository root.
func QuarkRoot(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve e2e utility caller path")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}
