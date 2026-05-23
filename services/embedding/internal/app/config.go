package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/quarkloop/pkg/serviceapi/servicebridge"
)

const defaultDimensions = 32
const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

type Config struct {
	Address           string
	SkillDir          string
	Provider          string
	Model             string
	Dimensions        int
	Fallbacks         []ProviderSpec
	OpenRouterAPIKey  string
	OpenRouterBaseURL string
	HTTPClient        *http.Client
	NATS              servicebridge.NATSConfig
	Logger            *slog.Logger
}

// ProviderSpec is one ordered embedding provider configuration.
type ProviderSpec struct {
	Provider   string
	Model      string
	Dimensions int
}

func normalizeProviderSpecs(primary ProviderSpec, fallbacks []ProviderSpec) []ProviderSpec {
	primary.Provider = normalizeProvider(primary.Provider)
	if primary.Provider == "" {
		primary.Provider = "local"
	}
	out := make([]ProviderSpec, 0, 1+len(fallbacks))
	seen := map[string]struct{}{}
	add := func(spec ProviderSpec) {
		spec.Provider = normalizeProvider(spec.Provider)
		spec.Model = strings.TrimSpace(spec.Model)
		if spec.Provider == "" {
			return
		}
		key := spec.Provider + "\x00" + spec.Model + "\x00" + fmt.Sprint(spec.Dimensions)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, spec)
	}
	add(primary)
	for _, fallback := range fallbacks {
		add(fallback)
	}
	return out
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// ParseProviderSpecs parses an ordered fallback string. Entries are comma
// separated and each entry uses provider|model|dimensions. Model and dimensions
// are optional, so "local" and "local||32" are both valid.
func ParseProviderSpecs(value string) ([]ProviderSpec, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	entries := strings.Split(value, ",")
	out := make([]ProviderSpec, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		if len(parts) > 3 {
			return nil, fmt.Errorf("invalid embedding provider spec %q: expected provider|model|dimensions", entry)
		}
		spec := ProviderSpec{Provider: strings.TrimSpace(parts[0])}
		if spec.Provider == "" {
			return nil, fmt.Errorf("invalid embedding provider spec %q: provider is required", entry)
		}
		if len(parts) > 1 {
			spec.Model = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
			dimensions, ok := parsePositiveInt(parts[2])
			if !ok {
				return nil, fmt.Errorf("invalid embedding provider spec %q: dimensions must be positive integer", entry)
			}
			spec.Dimensions = dimensions
		}
		out = append(out, spec)
	}
	return out, nil
}

func parsePositiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}
