package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/quarkloop/services/model/internal/app"
)

func main() {
	var addr string
	var skillDir string
	var fallbackSpec string
	flag.StringVar(&addr, "addr", "127.0.0.1:7306", "gRPC listen address")
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&fallbackSpec, "fallbacks", os.Getenv("QUARK_MODEL_FALLBACKS"), "fallbacks as provider=fallback1,fallback2;provider2=fallback")
	flag.Parse()

	fallbacks := parseFallbacks(fallbackSpec)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "model"),
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Config{
		Address:   addr,
		SkillDir:  skillDir,
		Providers: providerConfigsFromEnv(),
		Fallbacks: fallbacks,
		Logger:    logger,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func providerConfigsFromEnv() []app.ProviderConfig {
	configs := []app.ProviderConfig{{
		ID:      "local",
		Kind:    "local",
		Model:   envOrDefault("QUARK_LOCAL_MODEL", "local/noop"),
		Enabled: true,
	}}
	if key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); key != "" {
		configs = append(configs, app.ProviderConfig{
			ID:      "openrouter",
			Kind:    "openai-compatible",
			APIKey:  key,
			BaseURL: envOrDefault("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
			Model:   envOrDefault("OPENROUTER_MODEL", "openai/gpt-4o-mini"),
			Enabled: true,
		})
	}
	if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
		configs = append(configs, app.ProviderConfig{
			ID:      "openai",
			Kind:    "openai-compatible",
			APIKey:  key,
			BaseURL: envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			Model:   envOrDefault("OPENAI_MODEL", "gpt-4o-mini"),
			Enabled: true,
		})
	}
	if key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); key != "" {
		configs = append(configs, app.ProviderConfig{
			ID:      "anthropic",
			Kind:    "unsupported",
			APIKey:  key,
			Model:   envOrDefault("ANTHROPIC_MODEL", "claude-sonnet-4-5"),
			Enabled: true,
		})
	}
	if key := strings.TrimSpace(os.Getenv("ZHIPU_API_KEY")); key != "" {
		configs = append(configs, app.ProviderConfig{
			ID:      "zhipu",
			Kind:    "unsupported",
			APIKey:  key,
			Model:   envOrDefault("ZHIPU_MODEL", "glm-4.5"),
			Enabled: true,
		})
	}
	return configs
}

func parseFallbacks(spec string) map[string][]string {
	out := make(map[string][]string)
	for _, group := range strings.Split(spec, ";") {
		provider, rest, ok := strings.Cut(group, "=")
		provider = strings.TrimSpace(provider)
		if !ok || provider == "" {
			continue
		}
		for _, fallback := range strings.Split(rest, ",") {
			fallback = strings.TrimSpace(fallback)
			if fallback != "" {
				out[provider] = append(out[provider], fallback)
			}
		}
	}
	return out
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
