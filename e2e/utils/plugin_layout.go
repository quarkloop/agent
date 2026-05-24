//go:build e2e

package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func installSpacePlugins(t *testing.T, env *E2EEnv, bins BuiltBinaries, includeKnowledgeServices bool) {
	t.Helper()
	pluginsDir := filepath.Join(env.SpacesDir, env.Space, "plugins")
	srcRoot := filepath.Join(QuarkRoot(t), "plugins")

	// installTool lays out a tool plugin exactly the way production installs
	// do: manifest + the binary + (optionally) the lib-mode plugin.so. The
	// agent's pluginmanager prefers lib mode when the .so is present and
	// falls back to api mode otherwise, so shipping both proves both
	// code paths work.
	installAgent := func(name string) {
		src := filepath.Join(srcRoot, "agents", name)
		dst := filepath.Join(pluginsDir, "agents", name)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		for _, file := range []string{"manifest.yaml", "PROFILE.yaml", "SYSTEM.md", "SKILL.md"} {
			copyFile(t, filepath.Join(src, file), filepath.Join(dst, file), 0o644)
		}
	}
	for _, agent := range env.Agents {
		installAgent(agent)
	}

	installService := func(name string) {
		src := filepath.Join(srcRoot, "services", name)
		dst := filepath.Join(pluginsDir, "services", name)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		copyFile(t, filepath.Join(src, "manifest.yaml"), filepath.Join(dst, "manifest.yaml"), 0o644)
		copyFile(t, filepath.Join(src, "SKILL.md"), filepath.Join(dst, "SKILL.md"), 0o644)
		copyFile(t, filepath.Join(src, "README.md"), filepath.Join(dst, "README.md"), 0o644)
	}
	installService("io")
	if includeKnowledgeServices {
		installService("core")
		installService("gateway")
		installService("indexer")
		installService("document")
		installService("ingestion")
		installService("citation")
	}
	for _, service := range env.Services {
		installService(service.withDefaults().Plugin)
	}
	for _, service := range env.ExtraServicePlugins {
		installService(service)
	}
}

func copyFile(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
