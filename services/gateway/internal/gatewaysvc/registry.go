package gatewaysvc

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/quarkloop/pkg/plugin"
)

func (s *Server) resolveProvider(requested string) (string, provider, error) {
	chain, providers := s.providerChainSnapshot(requested)
	for _, providerID := range chain {
		p, ok := providers[providerID]
		if ok {
			return providerID, p, nil
		}
	}
	return "", nil, plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, requested, "", 0, fmt.Errorf("provider unavailable"))
}

func (s *Server) providerChainLocked(primary string) []string {
	primary = strings.TrimSpace(primary)
	if primary == "" {
		ids := s.providerIDsLocked()
		if len(ids) > 0 {
			primary = ids[0]
		}
	}
	chain := []string{primary}
	for _, fallback := range s.fallbacks[primary] {
		if fallback != "" && !contains(chain, fallback) {
			chain = append(chain, fallback)
		}
	}
	return chain
}

func (s *Server) providerChainSnapshot(primary string) ([]string, map[string]provider) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chain := s.providerChainLocked(primary)
	providers := make(map[string]provider, len(chain))
	for _, providerID := range chain {
		if p, ok := s.providers[providerID]; ok {
			providers[providerID] = p
		}
	}
	return chain, providers
}

func (s *Server) provider(providerID string) (provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[providerID]
	return p, ok
}

func (s *Server) Reload(configs []ProviderConfig, fallbacks map[string][]string) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("gateway server is not configured")
	}
	s.mu.RLock()
	merged := cloneProviderConfigMap(s.providerConfigs)
	s.mu.RUnlock()
	for _, cfg := range configs {
		cfg.ID = strings.TrimSpace(cfg.ID)
		if cfg.ID == "" {
			return nil, fmt.Errorf("provider id is required")
		}
		existing := merged[cfg.ID]
		if cfg.Kind == "" {
			cfg.Kind = existing.Kind
		}
		if cfg.APIKey == "" {
			cfg.APIKey = existing.APIKey
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = existing.BaseURL
		}
		if cfg.Model == "" {
			cfg.Model = existing.Model
		}
		if cfg.EmbeddingModel == "" {
			cfg.EmbeddingModel = existing.EmbeddingModel
		}
		merged[cfg.ID] = cfg
	}
	providers, providerConfigs, err := buildProviders(providerConfigMapValues(merged))
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	oldProviders := s.providers
	s.providers = providers
	s.providerConfigs = providerConfigs
	if fallbacks != nil {
		s.fallbacks = cloneFallbacks(fallbacks)
	}
	ids := s.providerIDsLocked()
	s.mu.Unlock()
	if err := closeProviders(oldProviders); err != nil {
		s.logger.Warn("close old gateway providers after reload", "error", err)
	}
	return ids, nil
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	providers := cloneProviderMap(s.providers)
	s.mu.RUnlock()
	return closeProviders(providers)
}

func newProvider(cfg ProviderConfig) (provider, error) {
	if strings.TrimSpace(cfg.ID) == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	switch strings.TrimSpace(cfg.Kind) {
	case "bifrost":
		return newBifrostProvider(cfg)
	case "openai-compatible":
		return newOpenAICompatibleProvider(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", cfg.Kind)
	}
}

func buildProviders(configs []ProviderConfig) (map[string]provider, map[string]ProviderConfig, error) {
	providers := make(map[string]provider)
	providerConfigs := make(map[string]ProviderConfig)
	for _, providerCfg := range configs {
		if !providerCfg.Enabled {
			continue
		}
		p, err := newProvider(providerCfg)
		if err != nil {
			return nil, nil, err
		}
		id := p.ID()
		providerCfg.ID = id
		providers[id] = p
		providerConfigs[id] = providerCfg
	}
	if len(providers) == 0 {
		return nil, nil, fmt.Errorf("at least one configured Gateway provider is required")
	}
	return providers, providerConfigs, nil
}

func cloneFallbacks(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for providerID, chain := range in {
		for _, fallback := range chain {
			if fallback != "" && !contains(out[providerID], fallback) {
				out[providerID] = append(out[providerID], fallback)
			}
		}
	}
	return out
}

func cloneProviderMap(in map[string]provider) map[string]provider {
	out := make(map[string]provider, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneProviderConfigMap(in map[string]ProviderConfig) map[string]ProviderConfig {
	out := make(map[string]ProviderConfig, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func providerConfigMapValues(in map[string]ProviderConfig) []ProviderConfig {
	out := make([]ProviderConfig, 0, len(in))
	for _, cfg := range in {
		out = append(out, cfg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func closeProviders(providers map[string]provider) error {
	var errs []error
	for _, provider := range providers {
		if closer, ok := provider.(closableProvider); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Server) providerIDsLocked() []string {
	out := make([]string, 0, len(s.providers))
	for id := range s.providers {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func contains(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}
