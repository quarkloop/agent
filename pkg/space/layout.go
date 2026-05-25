// Package space models the Space-service-owned on-disk space layout.
package space

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ConfigFile  = "space.json"
	PluginsDir  = "plugins"
	SessionsDir = "sessions"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Layout contains all filesystem paths for one space directory.
type Layout struct {
	Root string
	Name string
}

// NewLayout validates name and returns the layout rooted under spacesRoot.
func NewLayout(spacesRoot, name string) (Layout, error) {
	if err := ValidateName(name); err != nil {
		return Layout{}, err
	}
	return Layout{Root: filepath.Join(spacesRoot, name), Name: name}, nil
}

// ValidateName rejects names that could escape the spaces root or collide with
// path syntax. The config name is an identifier, not a path.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("space name is required")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("space name %q must not have leading or trailing whitespace", name)
	}
	if name == "." || name == ".." || filepath.Clean(name) != name {
		return fmt.Errorf("space name %q is not a valid directory name", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("space name %q must not contain path separators", name)
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("space name %q must contain only letters, numbers, dots, underscores, or hyphens", name)
	}
	return nil
}

func (l Layout) ConfigPath() string   { return filepath.Join(l.Root, ConfigFile) }
func (l Layout) PluginsPath() string  { return filepath.Join(l.Root, PluginsDir) }
func (l Layout) SessionsPath() string { return filepath.Join(l.Root, SessionsDir) }

// RequiredDirs returns the directories that must exist for a usable space.
func (l Layout) RequiredDirs() []string {
	return []string{l.Root, l.PluginsPath(), l.SessionsPath()}
}
