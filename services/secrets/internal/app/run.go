package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/quarkloop/pkg/serviceapi/observability"
	"github.com/quarkloop/services/secrets/internal/secretsnats"
	"github.com/quarkloop/services/secrets/internal/secretssvc"
)

type Config struct {
	Address        string
	SkillDir       string
	OpenBaoAddress string
	OpenBaoToken   string
	OpenBaoMount   string
	NATSURL        string
	NATSUser       string
	NATSPassword   string
	NATSQueue      string
	Audit          observability.RecorderConfig
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

	if cfg.NATSURL == "" {
		return fmt.Errorf("nats url is required")
	}
	natsServer := secretsnats.New(secretsnats.Config{
		URL:      cfg.NATSURL,
		Username: cfg.NATSUser,
		Password: cfg.NATSPassword,
		Queue:    cfg.NATSQueue,
		Logger:   cfg.Logger,
		Audit:    cfg.Audit,
	}, server)
	if err := natsServer.Start(ctx); err != nil {
		return err
	}
	defer natsServer.Close()
	cfg.Logger.Info("secrets service listening", "openbao", cfg.OpenBaoAddress)
	<-ctx.Done()
	return nil
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
	if cfg.NATSUser == "" {
		cfg.NATSUser = secretsnats.DefaultUser
	}
	if cfg.NATSQueue == "" {
		cfg.NATSQueue = secretsnats.DefaultQueue
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}
