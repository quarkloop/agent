package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/workflow/internal/workflowsvc"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

type Config struct {
	SkillDir          string
	TemporalAddress   string
	TemporalNamespace string
	TaskQueue         string
	NATS              natskit.Config
	Logger            *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	cfg = normalizeConfig(cfg)
	if cfg.NATS.URL == "" {
		return fmt.Errorf("nats url is required")
	}
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-workflow", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	events := workflowsvc.NewEventLog()

	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		return fmt.Errorf("connect temporal: %w", err)
	}
	workerInstance := worker.New(temporalClient, cfg.TaskQueue, worker.Options{})
	workflowsvc.RegisterTemporalWorker(workerInstance, events)
	if err := workerInstance.Start(); err != nil {
		temporalClient.Close()
		return fmt.Errorf("start temporal worker: %w", err)
	}

	engine, err := workflowsvc.NewTemporalEngine(temporalClient, cfg.TaskQueue, events)
	if err != nil {
		workerInstance.Stop()
		return err
	}
	server, err := workflowsvc.NewServer(engine, cfg.Logger)
	if err != nil {
		workerInstance.Stop()
		return err
	}
	defer server.Close()
	defer workerInstance.Stop()

	cfg.NATS.Logger = cfg.Logger
	cfg.Logger.Info("workflow service listening", "temporal", cfg.TemporalAddress, "task_queue", cfg.TaskQueue)
	return natskit.RunRPCService(ctx, cfg.NATS, workflowBinding(skill, server))
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

func normalizeConfig(cfg Config) Config {
	if cfg.TemporalAddress == "" {
		cfg.TemporalAddress = "127.0.0.1:7233"
	}
	if cfg.TemporalNamespace == "" {
		cfg.TemporalNamespace = "default"
	}
	if cfg.TaskQueue == "" {
		cfg.TaskQueue = "quark-workflow"
	}
	if cfg.NATS.Username == "" {
		cfg.NATS.Username = natskit.DefaultUser
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}
