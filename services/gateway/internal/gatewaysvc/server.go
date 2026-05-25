package gatewaysvc

import (
	"log/slog"
	"strings"
	"sync"
)

type Server struct {
	mu                sync.RWMutex
	providers         map[string]provider
	providerConfigs   map[string]ProviderConfig
	fallbacks         map[string][]string
	embeddingProvider string
	recorder          *usageRecorder
	logger            logger
}

func NewServer(cfg Config) (*Server, error) {
	providers, providerConfigs, err := buildProviders(cfg.Providers)
	if err != nil {
		return nil, err
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Server{
		providers:         providers,
		providerConfigs:   providerConfigs,
		fallbacks:         cloneFallbacks(cfg.Fallbacks),
		embeddingProvider: strings.TrimSpace(cfg.EmbeddingProvider),
		recorder:          newUsageRecorder(),
		logger:            cfg.Logger,
	}, nil
}

func (s *Server) ProviderIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providerIDsLocked()
}
