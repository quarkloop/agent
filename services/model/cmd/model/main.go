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
	"time"

	"github.com/quarkloop/services/model/internal/app"
)

func main() {
	var addr string
	var skillDir string
	var fallbackSpec string
	var natsURL string
	var natsUser string
	var natsPassword string
	var natsQueue string
	var natsTimeout time.Duration
	flag.StringVar(&addr, "addr", "127.0.0.1:7306", "service descriptor address")
	flag.StringVar(&skillDir, "skill-dir", "", "directory containing the service SKILL.md")
	flag.StringVar(&fallbackSpec, "fallbacks", os.Getenv("QUARK_MODEL_FALLBACKS"), "fallbacks as provider=fallback1,fallback2;provider2=fallback")
	flag.StringVar(&natsURL, "nats-url", os.Getenv("QUARK_NATS_URL"), "NATS URL for Gateway service-function endpoints")
	flag.StringVar(&natsUser, "nats-user", envOrDefault("QUARK_NATS_SERVICE_USER", os.Getenv("QUARK_NATS_USER")), "NATS username for Gateway service-function endpoints")
	flag.StringVar(&natsPassword, "nats-password", envOrDefault("QUARK_NATS_SERVICE_PASSWORD", os.Getenv("QUARK_NATS_PASSWORD")), "NATS password for Gateway service-function endpoints")
	flag.StringVar(&natsQueue, "nats-queue", envOrDefault("QUARK_GATEWAY_NATS_QUEUE", "q.gateway.v1"), "NATS queue group for Gateway service-function endpoints")
	flag.DurationVar(&natsTimeout, "nats-timeout", durationEnvOrDefault("QUARK_GATEWAY_TIMEOUT", 30*time.Second), "Gateway service-function request timeout")
	flag.Parse()

	fallbacks := parseFallbacks(fallbackSpec)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "service"),
		slog.String("service", "gateway"),
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Config{
		Address:  addr,
		SkillDir: skillDir,
		NATS: app.GatewayNATSConfig{
			URL:      natsURL,
			Username: natsUser,
			Password: natsPassword,
			Queue:    natsQueue,
			Timeout:  natsTimeout,
		},
		Providers: providerConfigsFromEnv(),
		Fallbacks: fallbacks,
		Logger:    logger,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func durationEnvOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func providerConfigsFromEnv() []app.ProviderConfig {
	configs := []app.ProviderConfig{{
		ID:      "local",
		Kind:    "local",
		Model:   envOrDefault("QUARK_LOCAL_MODEL", "local/noop"),
		Enabled: true,
	}}
	if key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); key != "" {
		kind := envOrDefault("QUARK_OPENROUTER_PROVIDER_KIND", "openai-compatible")
		configs = append(configs, app.ProviderConfig{
			ID:      "openrouter",
			Kind:    kind,
			APIKey:  key,
			BaseURL: envOrDefault("OPENROUTER_BASE_URL", openRouterBaseURL(kind)),
			Model:   envOrDefault("OPENROUTER_MODEL", "openai/gpt-4o-mini"),
			Enabled: true,
		})
	}
	if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
		configs = append(configs, app.ProviderConfig{
			ID:      "openai",
			Kind:    "bifrost",
			APIKey:  key,
			BaseURL: envOrDefault("OPENAI_BASE_URL", "https://api.openai.com"),
			Model:   envOrDefault("OPENAI_MODEL", "gpt-4o-mini"),
			Enabled: true,
		})
	}
	if key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); key != "" {
		configs = append(configs, app.ProviderConfig{
			ID:      "anthropic",
			Kind:    "bifrost",
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

func openRouterBaseURL(kind string) string {
	if strings.TrimSpace(kind) == "bifrost" {
		return "https://openrouter.ai/api"
	}
	return "https://openrouter.ai/api/v1"
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
