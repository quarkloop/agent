package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/model/internal/modelsvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Address   string
	SkillDir  string
	Providers []ProviderConfig
	Fallbacks map[string][]string
	Logger    *slog.Logger
}

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

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(cfg.Logger)))
	modelv1.RegisterModelServiceServer(grpcServer, server)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(modelv1.ModelService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	registry := servicekit.NewRegistry()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-model", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	if err := registry.Register(modelsvc.Descriptor(cfg.Address, skill)); err != nil {
		return err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)

	ln, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Address, err)
	}
	errCh := make(chan error, 1)
	go func() {
		cfg.Logger.Info("model service listening", "addr", cfg.Address, "providers", server.ProviderIDs())
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

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find model skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "model", "SKILL.md"), filepath.Join("services", "model", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("model service SKILL.md not found; pass --skill-dir")
}
