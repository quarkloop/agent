package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/quarkloop/pkg/serviceapi/observability"
	"github.com/quarkloop/services/secrets/internal/app"
)

func main() {
	var cfg app.Config
	flag.StringVar(&cfg.Address, "addr", envOrDefault("QUARK_SECRETS_ADDR", "127.0.0.1:7316"), "service descriptor address")
	flag.StringVar(&cfg.SkillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&cfg.OpenBaoAddress, "openbao-addr", envOrDefault("QUARK_OPENBAO_ADDR", "http://127.0.0.1:8200"), "OpenBao API address")
	flag.StringVar(&cfg.OpenBaoToken, "openbao-token", os.Getenv("QUARK_OPENBAO_TOKEN"), "OpenBao client token")
	flag.StringVar(&cfg.OpenBaoMount, "openbao-mount", envOrDefault("QUARK_OPENBAO_MOUNT", "secret"), "OpenBao KV v2 mount")
	flag.StringVar(&cfg.NATSURL, "nats-url", os.Getenv("QUARK_NATS_URL"), "NATS URL for service-function endpoints")
	flag.StringVar(&cfg.NATSUser, "nats-user", os.Getenv("QUARK_NATS_USER"), "NATS username")
	flag.StringVar(&cfg.NATSPassword, "nats-password", os.Getenv("QUARK_NATS_PASSWORD"), "NATS password")
	flag.StringVar(&cfg.NATSQueue, "nats-queue", os.Getenv("QUARK_SECRETS_NATS_QUEUE"), "NATS queue group")
	flag.Parse()
	cfg.Audit = observability.RecorderConfig{
		AuditPrefix:     os.Getenv("QUARK_NATS_AUDIT_PREFIX"),
		TelemetryPrefix: os.Getenv("QUARK_NATS_TELEMETRY_PREFIX"),
		Policy:          observability.DefaultAuditPolicy(),
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "secrets"),
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
