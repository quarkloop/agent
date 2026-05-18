package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/quarkloop/services/embedding/internal/app"
)

func main() {
	var addr string
	var skillDir string
	var provider string
	var model string
	var dimensions int
	var fallbacks string
	var openRouterBaseURL string
	flag.StringVar(&addr, "addr", "127.0.0.1:7304", "gRPC listen address")
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&provider, "provider", envOrDefault("QUARK_EMBEDDING_PROVIDER", "local"), "embedding provider: local or openrouter")
	flag.StringVar(&model, "model", os.Getenv("QUARK_EMBEDDING_MODEL"), "embedding model name")
	flag.IntVar(&dimensions, "dimensions", envInt("QUARK_EMBEDDING_DIMENSIONS"), "expected embedding dimensions")
	flag.StringVar(&fallbacks, "fallbacks", os.Getenv("QUARK_EMBEDDING_FALLBACKS"), "ordered fallback providers: provider|model|dimensions,provider|model|dimensions")
	flag.StringVar(&openRouterBaseURL, "openrouter-base-url", os.Getenv("OPENROUTER_BASE_URL"), "OpenRouter API base URL")
	flag.Parse()
	fallbackSpecs, err := app.ParseProviderSpecs(fallbacks)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "embedding"),
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Config{
		Address:           addr,
		SkillDir:          skillDir,
		Provider:          provider,
		Model:             model,
		Dimensions:        dimensions,
		Fallbacks:         fallbackSpecs,
		OpenRouterAPIKey:  os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterBaseURL: openRouterBaseURL,
		Logger:            logger,
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

func envInt(key string) int {
	value := os.Getenv(key)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}
