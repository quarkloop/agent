package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
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
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-gateway", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	cfg.NATS.Logger = cfg.Logger
	if cfg.Queue == "" {
		cfg.Queue = defaultGatewayQueue
	}
	cfg.Logger.Info("gateway service listening", "providers", server.ProviderIDs())
	return natskit.RunRPCService(ctx, cfg.NATS, cfg.Queue, gatewayBinding(cfg.Address, skill, server))
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

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find gateway skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "gateway", "SKILL.md"), filepath.Join("services", "gateway", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("gateway service SKILL.md not found; pass --skill-dir")
}
