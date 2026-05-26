//go:build e2e

package utils

import (
	"strings"
	"testing"
)

func TestComposeProjectNameIsIsolatedAndDockerSafe(t *testing.T) {
	name := composeProjectName("Test PDF / Provider:Usage")
	if !strings.HasPrefix(name, "quarke2e") || strings.ContainsAny(name, " /:") {
		t.Fatalf("invalid compose project name %q", name)
	}
}
