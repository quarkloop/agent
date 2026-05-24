package pluginbootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

func TestInstallerInstallsRequiredMainAgent(t *testing.T) {
	bundled := writeBundledMainAgent(t)
	bootstrap, err := New(bundled)
	if err != nil {
		t.Fatalf("new bootstrapper: %v", err)
	}
	plugins := pluginmanager.NewInstaller(filepath.Join(t.TempDir(), "plugins"))

	if err := bootstrap.InstallRequired(context.Background(), plugins); err != nil {
		t.Fatalf("install required plugin: %v", err)
	}
	installed, err := plugins.Get(MainAgentName)
	if err != nil {
		t.Fatalf("get installed plugin: %v", err)
	}
	if installed.Manifest.Type != plugin.TypeAgent {
		t.Fatalf("plugin type = %q", installed.Manifest.Type)
	}
}

func TestInstallerRejectsMissingBundle(t *testing.T) {
	bootstrap, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new bootstrapper: %v", err)
	}
	plugins := pluginmanager.NewInstaller(filepath.Join(t.TempDir(), "plugins"))
	if err := bootstrap.InstallRequired(context.Background(), plugins); err == nil {
		t.Fatal("missing required plugin unexpectedly installed")
	}
}

func writeBundledMainAgent(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "agents", MainAgentName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"manifest.yaml": "name: quark-main\nversion: \"1.0.0\"\ntype: agent\nmode: api\nagent:\n  profile: PROFILE.yaml\n  system: SYSTEM.md\n  skill: SKILL.md\n",
		"PROFILE.yaml":  "id: quark-main\nname: Quark Main\nrole: main\n",
		"SYSTEM.md":     "You are Quark Main.\n",
		"SKILL.md":      "Coordinate work.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
