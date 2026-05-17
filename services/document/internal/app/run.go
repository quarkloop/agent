package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/document/internal/docsvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Address   string
	SkillDir  string
	PDFToText string
	Logger    *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7307"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	documentServer := docsvc.NewServer(docsvc.Config{
		PDFToText: cfg.PDFToText,
		Logger:    cfg.Logger,
	})

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(cfg.Logger)))
	documentv1.RegisterDocumentServiceServer(grpcServer, documentServer)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(documentv1.DocumentService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	registry := servicekit.NewRegistry()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-document", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	if err := registry.Register(docsvc.Descriptor(cfg.Address, skill)); err != nil {
		return err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)

	ln, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Address, err)
	}
	errCh := make(chan error, 1)
	go func() {
		cfg.Logger.Info("document service listening", "addr", cfg.Address)
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
			return "", fmt.Errorf("find document skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "document", "SKILL.md"), filepath.Join("services", "document", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("document service SKILL.md not found; pass --skill-dir")
}
