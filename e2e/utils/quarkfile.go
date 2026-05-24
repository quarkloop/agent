//go:build e2e

package utils

import (
	"fmt"
	"strings"
)

func quarkfileFor(name, provider, model string, services []ServicePlugin, extraServicePlugins []string, agents []string, agentServices map[string][]string, includeKnowledgeServices bool) []byte {
	env := ""
	if provider != "noop" {
		env = `  env:
    - OPENROUTER_API_KEY
`
	}
	pluginRefs := ""
	seenPluginRefs := make(map[string]struct{})
	addPluginRef := func(ref string) {
		if ref == "" {
			return
		}
		if _, ok := seenPluginRefs[ref]; ok {
			return
		}
		seenPluginRefs[ref] = struct{}{}
		pluginRefs += fmt.Sprintf("  - ref: %s\n", ref)
	}
	serviceBlocks := ""
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
		serviceBlocks += fmt.Sprintf(`  - name: %s
    ref: quark/service-%s
    mode: %s
    address_env: %s
`, service.Name, service.Plugin, service.Mode, service.AddressEnv)
	}
	addService(ServicePlugin{Name: "io", Plugin: "io", Mode: "local", AddressEnv: "QUARK_IO_ADDR"})
	agentBlocks := ""
	for _, agent := range agents {
		addPluginRef("quark/agent-" + agent)
		agentBlocks += fmt.Sprintf(`  - profile: %s
    enabled: true
`, agent)
		if allowed, ok := agentServices[agent]; ok {
			if len(allowed) == 0 {
				agentBlocks += "    services: []\n"
			} else {
				agentBlocks += "    services:\n"
				for _, service := range allowed {
					agentBlocks += fmt.Sprintf("      - %s\n", service)
				}
			}
		}
	}
	agentsSection := ""
	if agentBlocks != "" {
		agentsSection = "agents:\n" + agentBlocks
	}
	if includeKnowledgeServices {
		addService(ServicePlugin{Name: "core", Plugin: "core", Mode: "local", AddressEnv: "QUARK_CORE_ADDR"})
		addService(ServicePlugin{Name: "gateway", Plugin: "gateway", Mode: "local", AddressEnv: "QUARK_GATEWAY_SERVICE_ADDR"})
		addService(ServicePlugin{Name: "indexer", Plugin: "indexer", Mode: "local", AddressEnv: "QUARK_INDEXER_ADDR"})
		addService(ServicePlugin{Name: "document", Plugin: "document", Mode: "local", AddressEnv: "QUARK_DOCUMENT_ADDR"})
		addService(ServicePlugin{Name: "runstate", Plugin: "runstate", Mode: "local", AddressEnv: "QUARK_RUNSTATE_ADDR"})
		addService(ServicePlugin{Name: "citation", Plugin: "citation", Mode: "local", AddressEnv: "QUARK_CITATION_ADDR"})
	}
	for _, service := range services {
		addService(service)
	}
	for _, plugin := range extraServicePlugins {
		addPluginRef("quark/service-" + plugin)
	}
	qf := fmt.Sprintf(`quark: "1.0"
meta:
  name: %s
  version: "0.1.0"
model:
  provider: %s
  name: %s
%s
plugins:
%s
%s
services:
%s`, name, provider, model, env, pluginRefs, agentsSection, serviceBlocks)
	return []byte(qf)
}

// ServicePlugin declares an additional service plugin for an e2e space.
type ServicePlugin struct {
	Name       string
	Plugin     string
	Mode       string
	AddressEnv string
}

func (s ServicePlugin) WithDefaults() ServicePlugin {
	return s.withDefaults()
}

func (s ServicePlugin) withDefaults() ServicePlugin {
	if s.Plugin == "" {
		s.Plugin = s.Name
	}
	if s.Mode == "" {
		s.Mode = "local"
	}
	if s.AddressEnv == "" && s.Name != "" {
		s.AddressEnv = "QUARK_" + strings.ToUpper(strings.ReplaceAll(s.Name, "-", "_")) + "_ADDR"
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

// startSupervisor launches a supervisor subprocess with an isolated spaces
