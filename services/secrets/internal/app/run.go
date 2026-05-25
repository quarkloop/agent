package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/secrets/internal/secretssvc"
)

type Config struct {
	Address        string
	SkillDir       string
	OpenBaoAddress string
	OpenBaoToken   string
	OpenBaoMount   string
	NATS           natskit.Config
	Logger         *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	cfg = normalizeConfig(cfg)
	if cfg.NATS.URL == "" {
		return fmt.Errorf("nats url is required")
	}
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-secrets", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	backend, err := secretssvc.NewOpenBaoClient(secretssvc.OpenBaoConfig{
		Address: cfg.OpenBaoAddress,
		Token:   cfg.OpenBaoToken,
		Mount:   cfg.OpenBaoMount,
	})
	if err != nil {
		return err
	}
	server, err := secretssvc.NewServer(backend, cfg.Logger)
	if err != nil {
		return err
	}

	cfg.NATS.Logger = cfg.Logger
	cfg.Logger.Info("secrets service listening", "openbao", cfg.OpenBaoAddress)
	return natskit.RunRPCService(ctx, cfg.NATS, natskit.Binding{
		Descriptor: secretssvc.Descriptor(cfg.Address, skill),
		Services: []natskit.RPCService{{
			Service:        "quark.secrets.v1.SecretsService",
			Implementation: server,
		}},
	})
}

func normalizeConfig(cfg Config) Config {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7316"
	}
	if cfg.OpenBaoAddress == "" {
		cfg.OpenBaoAddress = "http://127.0.0.1:8200"
	}
	if cfg.OpenBaoMount == "" {
		cfg.OpenBaoMount = "secret"
	}
	if cfg.NATS.Username == "" {
		cfg.NATS.Username = natskit.DefaultUser
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find secrets skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "secrets", "SKILL.md"), filepath.Join("services", "secrets", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("secrets service SKILL.md not found; pass --skill-dir")
}
