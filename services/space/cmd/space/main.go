package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/quarkloop/services/space/internal/app"
)

func main() {
	var addr string
	var rootDir string
	var skillDir string
	flag.StringVar(&addr, "addr", "127.0.0.1:7303", "gRPC listen address")
	flag.StringVar(&rootDir, "root", "", "space storage root")
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "space"),
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Config{
		Address:     addr,
		RootDir:     rootDir,
		SkillDir:    skillDir,
		Environment: os.Environ(),
		Logger:      logger,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
