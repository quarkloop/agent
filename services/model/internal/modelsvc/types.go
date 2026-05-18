package modelsvc

import (
	"context"
	"errors"
	"time"

	"github.com/quarkloop/pkg/plugin"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
)

type Config struct {
	Providers []ProviderConfig
	Fallbacks map[string][]string
	Logger    logger
}

type ProviderConfig struct {
	ID      string
	Kind    string
	APIKey  string
	BaseURL string
	Model   string
	Enabled bool
}

type logger interface {
	Warn(msg string, args ...any)
}

type provider interface {
	ID() string
	ListModels(context.Context) ([]*modelv1.ModelInfo, error)
	StreamGenerate(context.Context, generateCommand) (<-chan streamEvent, error)
	Embed(context.Context, embedCommand) ([]*modelv1.Embedding, error)
	Health(context.Context) providerHealth
}

type generateCommand struct {
	Model    string
	Messages []message
	Tools    []toolSchema
	Options  map[string]string
}

type embedCommand struct {
	Model      string
	Input      []string
	Dimensions int32
	Options    map[string]string
}

type message struct {
	Role       string
	Content    string
	ToolCalls  []toolCall
	ToolCallID string
}

type toolSchema struct {
	Name           string
	Description    string
	ParametersJSON string
}

type toolCall struct {
	Index         int32
	ID            string
	Type          string
	Name          string
	ArgumentsJSON string
}

type streamEvent struct {
	Delta     string
	ToolCalls []toolCall
	Done      bool
	Err       error
}

type providerHealth struct {
	Healthy bool
	Status  string
}

type modelUsage struct {
	Provider        string
	Model           string
	InputTokens     int64
	OutputTokens    int64
	EmbeddingTokens int64
	LatencyMillis   int64
	FallbackChain   []string
	RequestID       string
	FinishReason    string
}

func canFallbackAfter(err error) bool {
	var providerErr *plugin.ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	switch providerErr.Category {
	case plugin.ProviderErrorAuth,
		plugin.ProviderErrorRateLimit,
		plugin.ProviderErrorModelUnavailable,
		plugin.ProviderErrorContextOverflow,
		plugin.ProviderErrorTransport:
		return true
	default:
		return false
	}
}

func elapsedMillis(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}
