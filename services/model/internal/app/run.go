package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/quarkloop/services/model/internal/gatewaynats"
	"github.com/quarkloop/services/model/internal/modelsvc"
)

type Config struct {
	Address   string
	SkillDir  string
	NATS      gatewaynats.Config
	Providers []ProviderConfig
	Fallbacks map[string][]string
	Logger    *slog.Logger
}

type GatewayNATSConfig = gatewaynats.Config

type ProviderConfig struct {
	ID      string
	Kind    string
	APIKey  string
	BaseURL string
	Model   string
	Enabled bool
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7306"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	server, err := modelsvc.NewServer(modelsvc.Config{
		Providers: providerConfigs(cfg.Providers),
		Fallbacks: cfg.Fallbacks,
		Logger:    cfg.Logger,
	})
	if err != nil {
		return err
	}
	defer server.Close()

	if cfg.NATS.URL == "" {
		return fmt.Errorf("nats url is required")
	}
	cfg.NATS.Logger = cfg.Logger
	gateway := gatewaynats.New(cfg.NATS, server)
	if err := gateway.Start(ctx); err != nil {
		return err
	}
	defer gateway.Close()
	cfg.Logger.Info("gateway service listening", "providers", server.ProviderIDs())
	<-ctx.Done()
	return nil
}

func providerConfigs(in []ProviderConfig) []modelsvc.ProviderConfig {
	out := make([]modelsvc.ProviderConfig, 0, len(in))
	for _, cfg := range in {
		out = append(out, modelsvc.ProviderConfig{
			ID:      cfg.ID,
			Kind:    cfg.Kind,
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			Enabled: cfg.Enabled,
		})
	}
	return out
}
