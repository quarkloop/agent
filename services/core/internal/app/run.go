package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/core/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Address  string
	RootDir  string
	SkillDir string
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7305"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	root, err := resolveRoot(cfg.RootDir)
	if err != nil {
		return err
	}
	coreServer, err := server.New(root)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(cfg.Logger)))
	corev1.RegisterCoreServiceServer(grpcServer, coreServer)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(corev1.CoreService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	registry := servicekit.NewRegistry()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-core", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	if err := registry.Register(server.Descriptor(cfg.Address, skill)); err != nil {
		return err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)

	ln, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Address, err)
	}
	errCh := make(chan error, 1)
	go func() {
		cfg.Logger.Info("core service listening", "addr", cfg.Address, "root", root)
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

func resolveRoot(root string) (string, error) {
	if root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve config dir: %w", err)
		}
		root = filepath.Join(dir, "quarkloop", "core")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve core root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create core root: %w", err)
	}
	return abs, nil
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find core skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "core", "SKILL.md"), filepath.Join("services", "core", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("core service SKILL.md not found; pass --skill-dir")
}
