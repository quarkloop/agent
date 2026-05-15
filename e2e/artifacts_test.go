//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func writeArtifact(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write artifact %s: %v", path, err)
	}
	utils.Logf(t, "manual verification artifact: %s", path)
}

func writeTraceArtifact(t *testing.T, dir, name string, trace utils.MessageTrace) {
	t.Helper()
	payload := map[string]any{
		"text":    trace.Text,
		"starts":  trace.ToolStartEvents,
		"results": trace.ToolResultEvents,
	}
	writeJSONArtifact(t, dir, name, payload)
}

func writeJSONArtifact(t *testing.T, dir, name string, payload any) {
	t.Helper()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal artifact %s: %v", name, err)
	}
	writeArtifact(t, dir, name, string(data))
}
