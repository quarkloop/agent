package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/citation/internal/citationsvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Address  string
	SkillDir string
	NATS     servicebridge.NATSConfig
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7309"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	server := citationsvc.NewServer()
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(cfg.Logger)))
	citationv1.RegisterCitationServiceServer(grpcServer, server)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(citationv1.CitationService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	registry := servicekit.NewRegistry()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-citation", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := citationsvc.Descriptor(cfg.Address, skill)
	if err := registry.Register(descriptor); err != nil {
		return err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)
	if cfg.NATS.URL != "" {
		cfg.NATS.Logger = cfg.Logger
		natsService := servicebridge.NewNATSService(cfg.NATS)
		if err := natsService.Start(ctx, servicebridge.Binding{
			Descriptor: descriptor,
			Services: []servicebridge.GRPCService{{
				Desc:           &citationv1.CitationService_ServiceDesc,
				Implementation: server,
			}},
		}); err != nil {
			return err
		}
		defer natsService.Close()
	}

	ln, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Address, err)
	}
	errCh := make(chan error, 1)
	go func() {
		cfg.Logger.Info("citation service listening", "addr", cfg.Address)
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

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find citation skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "citation", "SKILL.md"), filepath.Join("services", "citation", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("citation service SKILL.md not found; pass --skill-dir")
}
