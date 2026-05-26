package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/services/citation/internal/app"
)

func main() {
	var skillDir string
	var natsURL string
	var natsUser string
	var natsPassword string
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&natsURL, "nats-url", os.Getenv("QUARK_NATS_URL"), "NATS URL for service-function endpoints")
	flag.StringVar(&natsUser, "nats-user", envOrDefault("QUARK_NATS_SERVICE_USER", os.Getenv("QUARK_NATS_USER")), "NATS username")
	flag.StringVar(&natsPassword, "nats-password", envOrDefault("QUARK_NATS_SERVICE_PASSWORD", os.Getenv("QUARK_NATS_PASSWORD")), "NATS password")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "citation"),
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Config{
		SkillDir: skillDir,
		NATS: natskit.Config{
			URL:             natsURL,
			Username:        natsUser,
			Password:        natsPassword,
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
