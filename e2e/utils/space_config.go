//go:build e2e

package utils

import (
	"testing"

	spacemodel "github.com/quarkloop/pkg/space"
)

func spaceConfigFor(t *testing.T, name, workingDir, provider, model string, services []ServicePlugin, extraServicePlugins []string, agents []string, agentServices map[string][]string, includeKnowledgeServices bool) []byte {
	t.Helper()
	config := spacemodel.NewConfig(name, workingDir)
	if provider != "" || model != "" {
		config.Model = spacemodel.Model{Provider: provider, Name: model}
		config.Model.Env = []string{"OPENROUTER_API_KEY"}
	}
	config.Plugins = nil
	seenPluginRefs := make(map[string]struct{})
	addPluginRef := func(ref string) {
		if ref == "" {
			return
		}
		if _, ok := seenPluginRefs[ref]; ok {
			return
		}
		seenPluginRefs[ref] = struct{}{}
		config.Plugins = append(config.Plugins, spacemodel.PluginRef{Ref: ref})
	}
	seenServices := make(map[string]struct{})
	addService := func(service ServicePlugin) {
		service = service.withDefaults()
		if service.Name == "" {
			return
		}
		if _, ok := seenServices[service.Name]; ok {
			return
		}
		seenServices[service.Name] = struct{}{}
		addPluginRef("quark/service-" + service.Plugin)
		config.Services = append(config.Services, spacemodel.ServiceRef{
			Name: service.Name,
			Ref:  "quark/service-" + service.Plugin,
		})
	}
	addService(ServicePlugin{Name: "io", Plugin: "io"})
	// Harness is runtime infrastructure: every agent inference turn delegates
	// context composition to it, regardless of the agent's callable services.
	addService(ServicePlugin{Name: "harness", Plugin: "harness"})
	enabled := true
	for _, agent := range agents {
		addPluginRef("quark/agent-" + agent)
		ref := spacemodel.AgentRef{Profile: agent, Enabled: &enabled}
		if allowed, ok := agentServices[agent]; ok {
			ref.Services = append([]string(nil), allowed...)
		}
		config.Agents = append(config.Agents, ref)
	}
	if includeKnowledgeServices {
		addService(ServicePlugin{Name: "core", Plugin: "core"})
		addService(ServicePlugin{Name: "gateway", Plugin: "gateway"})
		addService(ServicePlugin{Name: "indexer", Plugin: "indexer"})
		addService(ServicePlugin{Name: "document", Plugin: "document"})
		addService(ServicePlugin{Name: "runstate", Plugin: "runstate"})
		addService(ServicePlugin{Name: "citation", Plugin: "citation"})
	}
	for _, service := range services {
		addService(service)
	}
	for _, plugin := range extraServicePlugins {
		addPluginRef("quark/service-" + plugin)
	}
	data, err := spacemodel.MarshalConfig(config)
	if err != nil {
		t.Fatalf("marshal space config: %v", err)
	}
	return data
}

// ServicePlugin declares an additional service plugin for an e2e space.
type ServicePlugin struct {
	Name   string
	Plugin string
}

func (s ServicePlugin) WithDefaults() ServicePlugin {
	return s.withDefaults()
}

func (s ServicePlugin) withDefaults() ServicePlugin {
	if s.Plugin == "" {
		s.Plugin = s.Name
	}
	return s
}

// GatewayEmbeddingOptions configures the real Gateway embedding provider used
// by a knowledge E2E. It does not create a separate embedding service.
type GatewayEmbeddingOptions struct {
	Provider   string
	Model      string
	Dimensions int
}

// WithDefaults returns Gateway embedding configuration for test artifacts and
// the Gateway service process environment.
func (o GatewayEmbeddingOptions) WithDefaults() GatewayEmbeddingOptions {
	return o.withDefaults()
}

func (o GatewayEmbeddingOptions) withDefaults() GatewayEmbeddingOptions {
	if o.Provider == "" {
		o.Provider = "openrouter"
	}
	return o
}
