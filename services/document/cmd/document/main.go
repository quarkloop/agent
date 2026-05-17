package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/quarkloop/services/document/internal/app"
)

func main() {
	var addr string
	var skillDir string
	var pdftotextPath string
	flag.StringVar(&addr, "addr", "127.0.0.1:7307", "gRPC listen address")
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&pdftotextPath, "pdftotext", os.Getenv("QUARK_PDFTOTEXT_PATH"), "pdftotext executable path; empty resolves from PATH")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Config{
		Address:   addr,
		SkillDir:  skillDir,
		PDFToText: pdftotextPath,
		Logger:    logger,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
