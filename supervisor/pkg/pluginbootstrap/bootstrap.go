// Package pluginbootstrap installs product-required plugins into newly
// created spaces from the supervisor's configured bundled plugin directory.
package pluginbootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

const MainAgentName = "quark-main"

type Installer struct {
	bundledPluginsDir string
}

func New(bundledPluginsDir string) (*Installer, error) {
	root := strings.TrimSpace(bundledPluginsDir)
	if root == "" {
		return nil, fmt.Errorf("bundled plugins directory is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve bundled plugins directory: %w", err)
	}
	return &Installer{bundledPluginsDir: filepath.Clean(absoluteRoot)}, nil
}

func (i *Installer) InstallRequired(ctx context.Context, plugins *pluginmanager.Installer) error {
	if i == nil {
		return fmt.Errorf("required plugin bootstrapper is not configured")
	}
	if plugins == nil {
		return fmt.Errorf("space plugin installer is required")
	}
	if installed, err := plugins.Get(MainAgentName); err == nil {
		if installed.Manifest == nil || installed.Manifest.Type != plugin.TypeAgent {
			return fmt.Errorf("required plugin %q exists with an invalid plugin type", MainAgentName)
		}
		return nil
	}
	source := filepath.Join(i.bundledPluginsDir, "agents", MainAgentName)
	installed, err := plugins.Install(ctx, source)
	if err != nil {
		return fmt.Errorf("install required main agent plugin from %s: %w", source, err)
	}
	if installed.Manifest == nil || installed.Manifest.Name != MainAgentName || installed.Manifest.Type != plugin.TypeAgent {
		return fmt.Errorf("bundled plugin %q is not the required main agent", source)
	}
	return nil
}
