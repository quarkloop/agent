package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	natsgo "github.com/nats-io/nats.go"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/workflow/internal/workflownats"
	"github.com/quarkloop/services/workflow/internal/workflowsvc"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Address           string
	SkillDir          string
	TemporalAddress   string
	TemporalNamespace string
	TaskQueue         string
	NATSURL           string
	NATSUser          string
	NATSPassword      string
	NATSQueue         string
	Logger            *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	cfg = normalizeConfig(cfg)
	events := workflowsvc.NewEventLog()

	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		return fmt.Errorf("connect temporal: %w", err)
	}
	var serviceConn *natsgo.Conn
	if cfg.NATSURL != "" {
		serviceConn, err = natsgo.Connect(cfg.NATSURL, natsgo.UserInfo(cfg.NATSUser, cfg.NATSPassword), natsgo.Name("quark-workflow-activities"))
		if err != nil {
			temporalClient.Close()
			return fmt.Errorf("connect nats for workflow activities: %w", err)
		}
	}
	dispatcher := workflowsvc.NewNATSDispatcher(serviceConn, workflownats.DefaultTimeout)
	workerInstance := worker.New(temporalClient, cfg.TaskQueue, worker.Options{})
	workflowsvc.RegisterTemporalWorker(workerInstance, dispatcher, events)
	if err := workerInstance.Start(); err != nil {
		if serviceConn != nil {
			serviceConn.Close()
		}
		temporalClient.Close()
		return fmt.Errorf("start temporal worker: %w", err)
	}

	engine, err := workflowsvc.NewTemporalEngine(temporalClient, cfg.TaskQueue, events)
	if err != nil {
		workerInstance.Stop()
		if serviceConn != nil {
			serviceConn.Close()
		}
		return err
	}
	server, err := workflowsvc.NewServer(engine, cfg.Logger)
	if err != nil {
		workerInstance.Stop()
		if serviceConn != nil {
			serviceConn.Close()
		}
		return err
	}
	defer server.Close()
	defer workerInstance.Stop()
	defer func() {
		if serviceConn != nil {
			serviceConn.Drain()
			serviceConn.Close()
		}
	}()

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(cfg.Logger)))
	workflowv1.RegisterWorkflowServiceServer(grpcServer, server)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(workflowv1.WorkflowService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	registry := servicekit.NewRegistry()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-workflow", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	if err := registry.Register(workflowsvc.Descriptor(cfg.Address, skill)); err != nil {
		return err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)

	if cfg.NATSURL != "" {
		natsServer := workflownats.New(workflownats.Config{
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
		cfg.Logger.Info("workflow service listening", "addr", cfg.Address, "temporal", cfg.TemporalAddress, "task_queue", cfg.TaskQueue)
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
		cfg.Address = "127.0.0.1:7315"
	}
	if cfg.TemporalAddress == "" {
		cfg.TemporalAddress = "127.0.0.1:7233"
	}
	if cfg.TemporalNamespace == "" {
		cfg.TemporalNamespace = "default"
	}
	if cfg.TaskQueue == "" {
		cfg.TaskQueue = "quark-workflow"
	}
	if cfg.NATSUser == "" {
		cfg.NATSUser = workflownats.DefaultUser
	}
	if cfg.NATSQueue == "" {
		cfg.NATSQueue = workflownats.DefaultQueue
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
			return "", fmt.Errorf("find workflow skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "workflow", "SKILL.md"), filepath.Join("services", "workflow", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("workflow service SKILL.md not found; pass --skill-dir")
}
