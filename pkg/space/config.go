package space

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const ConfigSchema = "quark.space/v1"

// Config is the authoritative persisted representation of one space. Plugin
// profile defaults remain plugin-owned; this record contains only space
// identity and space-selected overrides.
type Config struct {
	Schema       string            `json:"schema"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Version      string            `json:"version"`
	Author       string            `json:"author,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	WorkingDir   string            `json:"working_dir,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Model        Model             `json:"model,omitempty"`
	Routing      RoutingSection    `json:"routing,omitempty"`
	Plugins      []PluginRef       `json:"plugins"`
	Agents       []AgentRef        `json:"agents,omitempty"`
	Services     []ServiceRef      `json:"services,omitempty"`
	Permissions  Permissions       `json:"permissions,omitempty"`
	Capabilities Capabilities      `json:"capabilities,omitempty"`
	Gateway      Gateway           `json:"gateway,omitempty"`
}

// NewConfig creates a new service-owned space config with stable defaults.
func NewConfig(name, workingDir string) *Config {
	now := time.Now().UTC()
	return &Config{
		Schema:     ConfigSchema,
		Name:       name,
		Version:    "0.1.0",
		WorkingDir: workingDir,
		CreatedAt:  now,
		UpdatedAt:  now,
		Plugins:    []PluginRef{{Ref: "quark/tool-fs"}},
	}
}

func (c *Config) EnvironmentVariables() []string {
	if c == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(c.Model.Env))
	add := func(names []string) {
		for _, name := range names {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	add(c.Model.Env)
	for _, agent := range c.Agents {
		add(agent.Model.Env)
	}
	return out
}

func (c *Config) DefaultModel() (Model, bool) {
	if c == nil || c.Model.IsZero() {
		return Model{}, false
	}
	return c.Model, true
}

func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse space config: %w", err)
	}
	return &cfg, nil
}

func ParseAndValidateConfig(data []byte, spaceName string) (*Config, error) {
	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, err
	}
	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid space config: %w", err)
	}
	if cfg.Name != spaceName {
		return nil, fmt.Errorf("space config name %q does not match space name %q", cfg.Name, spaceName)
	}
	return cfg, nil
}

func MarshalConfig(cfg *Config) ([]byte, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal space config: %w", err)
	}
	return append(data, '\n'), nil
}

func ReadConfigFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("read space config: %w", err)
	}
	return data, nil
}

func WriteConfigFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create space config dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write space config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename space config: %w", err)
	}
	return nil
}
