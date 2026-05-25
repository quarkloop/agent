package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/services/gateway/internal/gatewaysvc"
)

type Config struct {
	Address           string
	SkillDir          string
	NATS              natskit.Config
	Queue             string
	Providers         []ProviderConfig
	Fallbacks         map[string][]string
	EmbeddingProvider string
	Logger            *slog.Logger
}

type ProviderConfig struct {
	ID             string
	Kind           string
	APIKey         string
	BaseURL        string
	Model          string
	EmbeddingModel string
	Enabled        bool
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7306"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	server, err := gatewaysvc.NewServer(gatewaysvc.Config{
		Providers:         providerConfigs(cfg.Providers),
		Fallbacks:         cfg.Fallbacks,
		EmbeddingProvider: cfg.EmbeddingProvider,
		Logger:            cfg.Logger,
	})
	if err != nil {
		return err
	}
	defer server.Close()

	if cfg.NATS.URL == "" {
		return fmt.Errorf("nats url is required")
	}
	cfg.NATS.Logger = cfg.Logger
	host, err := startGatewayHost(ctx, cfg.NATS, cfg.Queue, server)
	if err != nil {
		return err
	}
	defer host.Close()
	cfg.Logger.Info("gateway service listening", "providers", server.ProviderIDs())
	<-ctx.Done()
	return nil
}

func providerConfigs(in []ProviderConfig) []gatewaysvc.ProviderConfig {
	out := make([]gatewaysvc.ProviderConfig, 0, len(in))
	for _, cfg := range in {
		out = append(out, gatewaysvc.ProviderConfig{
			ID:             cfg.ID,
			Kind:           cfg.Kind,
			APIKey:         cfg.APIKey,
			BaseURL:        cfg.BaseURL,
			Model:          cfg.Model,
			EmbeddingModel: cfg.EmbeddingModel,
			Enabled:        cfg.Enabled,
		})
	}
	return out
}
