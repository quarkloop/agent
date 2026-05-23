package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/quarkloop/services/workflow/internal/app"
)

func main() {
	var cfg app.Config
	flag.StringVar(&cfg.Address, "addr", envOrDefault("QUARK_WORKFLOW_ADDR", "127.0.0.1:7315"), "gRPC listen address")
	flag.StringVar(&cfg.SkillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&cfg.TemporalAddress, "temporal-addr", envOrDefault("QUARK_TEMPORAL_ADDR", "127.0.0.1:7233"), "Temporal frontend address")
	flag.StringVar(&cfg.TemporalNamespace, "temporal-namespace", envOrDefault("QUARK_TEMPORAL_NAMESPACE", "default"), "Temporal namespace")
	flag.StringVar(&cfg.TaskQueue, "task-queue", envOrDefault("QUARK_WORKFLOW_TASK_QUEUE", "quark-workflow"), "Temporal task queue")
	flag.StringVar(&cfg.NATSURL, "nats-url", os.Getenv("QUARK_NATS_URL"), "NATS URL for service-function endpoints and workflow activities")
	flag.StringVar(&cfg.NATSUser, "nats-user", os.Getenv("QUARK_NATS_USER"), "NATS username")
	flag.StringVar(&cfg.NATSPassword, "nats-password", os.Getenv("QUARK_NATS_PASSWORD"), "NATS password")
	flag.StringVar(&cfg.NATSQueue, "nats-queue", os.Getenv("QUARK_WORKFLOW_NATS_QUEUE"), "NATS queue group")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "workflow"),
	}))
	cfg.Logger = logger
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
