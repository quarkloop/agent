package plugin

import "fmt"

const RuntimeCatalogVersion = 1

// RuntimeCatalog is the supervisor-resolved plugin catalog consumed by runtime.
type RuntimeCatalog struct {
	Version int                    `json:"version"`
	Plugins []RuntimeCatalogPlugin `json:"plugins"`
}

// RuntimeCatalogPlugin is one resolved plugin entry in the runtime startup
// contract.
type RuntimeCatalogPlugin struct {
	Name         string        `json:"name"`
	Type         PluginType    `json:"type"`
	Path         string        `json:"path"`
	Schema       *ToolSchema   `json:"schema,omitempty"`
	Skill        string        `json:"skill,omitempty"`
	AgentProfile *AgentProfile `json:"agent_profile,omitempty"`
	SystemPrompt string        `json:"system_prompt,omitempty"`
}

// NewRuntimeCatalog builds a versioned runtime catalog with copied entries.
func NewRuntimeCatalog(entries []RuntimeCatalogPlugin) RuntimeCatalog {
	out := RuntimeCatalog{
		Version: RuntimeCatalogVersion,
		Plugins: make([]RuntimeCatalogPlugin, len(entries)),
	}
	copy(out.Plugins, entries)
	return out
}

func (c RuntimeCatalog) Empty() bool {
	return len(c.Plugins) == 0
}

func (c RuntimeCatalog) Validate() error {
	if c.Version != RuntimeCatalogVersion {
		return fmt.Errorf("unsupported runtime plugin catalog version %d (supported: %d)", c.Version, RuntimeCatalogVersion)
	}
	for i, item := range c.Plugins {
		if item.Name == "" {
			return fmt.Errorf("plugins[%d]: missing name", i)
		}
		if item.Path == "" {
			return fmt.Errorf("plugins[%d] %q: missing path", i, item.Name)
		}
		switch item.Type {
		case TypeTool:
			if item.Schema == nil {
				return fmt.Errorf("plugins[%d] %q: tool schema is required", i, item.Name)
			}
			if item.Schema.Name == "" {
				return fmt.Errorf("plugins[%d] %q: tool schema name is required", i, item.Name)
			}
		case TypeAgent:
			if item.AgentProfile == nil {
				return fmt.Errorf("plugins[%d] %q: agent profile is required", i, item.Name)
			}
			if err := item.AgentProfile.Validate(); err != nil {
				return fmt.Errorf("plugins[%d] %q: %w", i, item.Name, err)
			}
		default:
			return fmt.Errorf("plugins[%d] %q: unsupported plugin type %q", i, item.Name, item.Type)
		}
	}
	return nil
}
