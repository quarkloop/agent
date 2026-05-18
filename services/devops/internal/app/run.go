package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/devops/internal/devopssvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Address  string
	SkillDir string
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7310"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	server := devopssvc.NewServer()

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(cfg.Logger)))
	devopsv1.RegisterRepoServiceServer(grpcServer, server)
	devopsv1.RegisterBuildServiceServer(grpcServer, server)
	devopsv1.RegisterTestServiceServer(grpcServer, server)
	devopsv1.RegisterContainerServiceServer(grpcServer, server)
	devopsv1.RegisterDeployServiceServer(grpcServer, server)
	devopsv1.RegisterPolicyServiceServer(grpcServer, server)

	healthServer := health.NewServer()
	for _, service := range []string{
		devopsv1.RepoService_ServiceDesc.ServiceName,
		devopsv1.BuildService_ServiceDesc.ServiceName,
		devopsv1.TestService_ServiceDesc.ServiceName,
		devopsv1.ContainerService_ServiceDesc.ServiceName,
		devopsv1.DeployService_ServiceDesc.ServiceName,
		devopsv1.PolicyService_ServiceDesc.ServiceName,
	} {
		healthServer.SetServingStatus(service, healthpb.HealthCheckResponse_SERVING)
	}
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	registry := servicekit.NewRegistry()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-devops", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	if err := registry.Register(devopssvc.Descriptor(cfg.Address, skill)); err != nil {
		return err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)

	ln, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Address, err)
	}
	errCh := make(chan error, 1)
	go func() {
		cfg.Logger.Info("devops service listening", "addr", cfg.Address)
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
			return "", fmt.Errorf("find devops skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "devops", "SKILL.md"), filepath.Join("services", "devops", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("devops service SKILL.md not found; pass --skill-dir")
}
