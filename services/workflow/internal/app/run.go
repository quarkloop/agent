package app

import (
	"context"
	"fmt"
	"log/slog"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/observability"
	"github.com/quarkloop/services/workflow/internal/workflownats"
	"github.com/quarkloop/services/workflow/internal/workflowsvc"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
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
	Audit             observability.RecorderConfig
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

	if cfg.NATSURL == "" {
		return fmt.Errorf("nats url is required")
	}
	natsServer := workflownats.New(workflownats.Config{
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
	cfg.Logger.Info("workflow service listening", "temporal", cfg.TemporalAddress, "task_queue", cfg.TaskQueue)
	<-ctx.Done()
	return nil
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
