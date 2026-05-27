//go:build e2e

package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeProjectNameIsIsolatedAndDockerSafe(t *testing.T) {
	name := composeProjectName("Test PDF / Provider:Usage")
	if !strings.HasPrefix(name, "quarke2e") || strings.ContainsAny(name, " /:") {
		t.Fatalf("invalid compose project name %q", name)
	}
}

func TestWorkspaceContainerUserIsDefinedForBindMountedFixtures(t *testing.T) {
	if got := strings.TrimSpace(workspaceContainerUser()); got == "" {
		t.Fatal("workspace container user is empty")
	}
}

func TestCopyArtifactDirectoryPreservesInspectableFailureOutput(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "preserved")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "trace.json"), []byte(`{"status":"failed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyArtifactDirectory(source, destination); err != nil {
		t.Fatalf("copy artifacts: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "nested", "trace.json"))
	if err != nil || string(data) != `{"status":"failed"}` {
		t.Fatalf("preserved artifact = %q, error = %v", data, err)
	}
}
