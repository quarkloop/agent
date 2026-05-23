package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/services/citation/internal/app"
)

func main() {
	var addr string
	var skillDir string
	var natsURL string
	var natsUser string
	var natsPassword string
	var natsQueue string
	flag.StringVar(&addr, "addr", "127.0.0.1:7309", "service descriptor address")
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&natsURL, "nats-url", os.Getenv("QUARK_NATS_URL"), "NATS URL for service-function endpoints")
	flag.StringVar(&natsUser, "nats-user", envOrDefault("QUARK_NATS_SERVICE_USER", os.Getenv("QUARK_NATS_USER")), "NATS username")
	flag.StringVar(&natsPassword, "nats-password", envOrDefault("QUARK_NATS_SERVICE_PASSWORD", os.Getenv("QUARK_NATS_PASSWORD")), "NATS password")
	flag.StringVar(&natsQueue, "nats-queue", envOrDefault("QUARK_CITATION_NATS_QUEUE", "q.citation.v1"), "NATS queue group")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "citation"),
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Config{
		Address:  addr,
		SkillDir: skillDir,
		NATS: servicebridge.NATSConfig{
			URL:             natsURL,
			Username:        natsUser,
			Password:        natsPassword,
			Queue:           natsQueue,
			Name:            "quark-citation",
			AuditPrefix:     os.Getenv("QUARK_NATS_AUDIT_PREFIX"),
			TelemetryPrefix: os.Getenv("QUARK_NATS_TELEMETRY_PREFIX"),
		},
		Logger: logger,
	}); err != nil {
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
