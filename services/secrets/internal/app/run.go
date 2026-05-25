package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/services/secrets/internal/secretssvc"
)

type Config struct {
	Address        string
	SkillDir       string
	OpenBaoAddress string
	OpenBaoToken   string
	OpenBaoMount   string
	NATS           natskit.Config
	Queue          string
	Logger         *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	cfg = normalizeConfig(cfg)
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

	if cfg.NATS.URL == "" {
		return fmt.Errorf("nats url is required")
	}
	cfg.NATS.Logger = cfg.Logger
	cfg.Logger.Info("secrets service listening", "openbao", cfg.OpenBaoAddress)
	return natskit.RunRPCService(ctx, cfg.NATS, cfg.Queue, natskit.Binding{
		Descriptor: secretssvc.Descriptor(cfg.Address, nil),
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
	if cfg.Queue == "" {
		cfg.Queue = "q.secrets.v1"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}
