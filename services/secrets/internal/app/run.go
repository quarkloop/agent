package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/secrets/internal/secretsnats"
	"github.com/quarkloop/services/secrets/internal/secretssvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(cfg.Logger)))
	secretsv1.RegisterSecretsServiceServer(grpcServer, server)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(secretsv1.SecretsService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	registry := servicekit.NewRegistry()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-secrets", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	if err := registry.Register(secretssvc.Descriptor(cfg.Address, skill)); err != nil {
		return err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)

	if cfg.NATSURL != "" {
		natsServer := secretsnats.New(secretsnats.Config{
			URL:      cfg.NATSURL,
			Username: cfg.NATSUser,
			Password: cfg.NATSPassword,
			Queue:    cfg.NATSQueue,
			Logger:   cfg.Logger,
		}, server)
		if err := natsServer.Start(ctx); err != nil {
			return err
		}
		defer natsServer.Close()
	}

	ln, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Address, err)
	}
	errCh := make(chan error, 1)
	go func() {
		cfg.Logger.Info("secrets service listening", "addr", cfg.Address, "openbao", cfg.OpenBaoAddress)
		errCh <- grpcServer.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
		return nil
	case err := <-errCh:
		return err
	}
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
