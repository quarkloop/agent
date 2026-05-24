//go:build e2e

package utils

import (
	"fmt"
	"strings"
)

func quarkfileFor(name, provider, model string, embedding EmbeddingOptions, services []ServicePlugin, extraServicePlugins []string, agents []string, agentServices map[string][]string, includeKnowledgeServices bool) []byte {
	env := ""
	if provider != "noop" {
		env = `  env:
    - OPENROUTER_API_KEY
`
	}
	embedding = embedding.withDefaults()
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
	embeddingBlock := ""
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
		addService(ServicePlugin{Name: "ingestion", Plugin: "ingestion", Mode: "local", AddressEnv: "QUARK_INGESTION_ADDR"})
		addService(ServicePlugin{Name: "citation", Plugin: "citation", Mode: "local", AddressEnv: "QUARK_CITATION_ADDR"})
		addService(ServicePlugin{Name: "embedding", Plugin: embedding.Plugin, Mode: embedding.Mode, AddressEnv: "QUARK_EMBEDDING_ADDR"})
		embeddingBlock = fmt.Sprintf(`embedding:
  service: embedding
  provider: %s
  model: %s
  dimensions: %d
`, embedding.Provider, embedding.Model, embedding.Dimensions)
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
%s
%s`, name, provider, model, env, pluginRefs, agentsSection, serviceBlocks, embeddingBlock)
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

// EmbeddingOptions selects which embedding service plugin/profile the e2e
// space declares. The service process must be started by the test hook.
type EmbeddingOptions struct {
	Plugin     string
	Mode       string
	Provider   string
	Model      string
	Dimensions int
}

// WithDefaults returns a fully populated embedding profile for callers outside
// the utils package that need to start the matching service process.
func (o EmbeddingOptions) WithDefaults() EmbeddingOptions {
	return o.withDefaults()
}

func (o EmbeddingOptions) withDefaults() EmbeddingOptions {
	if o.Plugin == "" {
		o.Plugin = "embedding"
	}
	if o.Mode == "" {
		o.Mode = "local"
	}
	if o.Provider == "" {
		o.Provider = "local"
	}
	if o.Model == "" {
		o.Model = "local-hash-v1"
	}
	if o.Dimensions == 0 {
		o.Dimensions = 32
	}
	return o
}

// startSupervisor launches a supervisor subprocess with an isolated spaces
