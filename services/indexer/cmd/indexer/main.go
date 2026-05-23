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
	"github.com/quarkloop/services/indexer/internal/app"
	"github.com/quarkloop/services/indexer/internal/dgraph"
)

func main() {
	var addr string
	var dgraphAddr string
	var skillDir string
	var natsURL string
	var natsUser string
	var natsPassword string
	var natsQueue string
	flag.StringVar(&addr, "addr", "127.0.0.1:7301", "service descriptor address")
	flag.StringVar(&dgraphAddr, "dgraph", "127.0.0.1:9080", "Dgraph Alpha gRPC address")
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&natsURL, "nats-url", os.Getenv("QUARK_NATS_URL"), "NATS URL for service-function endpoints")
	flag.StringVar(&natsUser, "nats-user", envOrDefault("QUARK_NATS_SERVICE_USER", os.Getenv("QUARK_NATS_USER")), "NATS username")
	flag.StringVar(&natsPassword, "nats-password", envOrDefault("QUARK_NATS_SERVICE_PASSWORD", os.Getenv("QUARK_NATS_PASSWORD")), "NATS password")
	flag.StringVar(&natsQueue, "nats-queue", envOrDefault("QUARK_INDEXER_NATS_QUEUE", "q.indexer.v1"), "NATS queue group")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "indexer"),
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	driver, err := dgraph.New(ctx, dgraph.Config{
		Address: dgraphAddr,
		Logger:  logger,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := app.Run(ctx, app.Config{
		Address:  addr,
		Driver:   driver,
		SkillDir: skillDir,
		NATS: servicebridge.NATSConfig{
			URL:             natsURL,
			Username:        natsUser,
			Password:        natsPassword,
			Queue:           natsQueue,
			Name:            "quark-indexer",
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
