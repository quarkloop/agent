package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/quarkloop/services/ingestion/internal/app"
)

func main() {
	var addr string
	var root string
	var skillDir string
	flag.StringVar(&addr, "addr", "127.0.0.1:7308", "gRPC listen address")
	flag.StringVar(&root, "root", os.Getenv("QUARK_INGESTION_ROOT"), "Ingestion service state directory")
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Config{
		Address:  addr,
		RootDir:  root,
		SkillDir: skillDir,
		Logger:   logger,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
