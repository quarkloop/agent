//go:build e2e

package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDumpNATSCLIDiagnosticsWritesInspectableArtifacts(t *testing.T) {
	script := filepath.Join(t.TempDir(), "nats")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'diagnostic-output\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake nats CLI: %v", err)
	}
	t.Setenv("NATS_CLI", script)
	dir := t.TempDir()
	DumpNATSCLIDiagnostics(t, NATSEndpoints{ClientURL: "nats://test.invalid:4222"}, "unit", dir)
	data, err := os.ReadFile(filepath.Join(dir, "nats-unit-account-info.txt"))
	if err != nil {
		t.Fatalf("read NATS diagnostic artifact: %v", err)
	}
	if !strings.Contains(string(data), "diagnostic-output") {
		t.Fatalf("NATS diagnostic artifact missing probe output: %s", data)
	}
}
