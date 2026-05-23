package modelsvc

import (
	"context"
	"errors"
	"sync"
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
	Usage     *modelUsage
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
	CostEstimate    float64
	FallbackChain   []string
	RequestID       string
	FinishReason    string
}

type StreamGenerateEvent struct {
	Response *modelv1.StreamGenerateResponse
	Err      error
}

type UsageAggregate struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model,omitempty"`
	Requests        int64    `json:"requests"`
	InputTokens     int64    `json:"input_tokens,omitempty"`
	OutputTokens    int64    `json:"output_tokens,omitempty"`
	EmbeddingTokens int64    `json:"embedding_tokens,omitempty"`
	TotalTokens     int64    `json:"total_tokens,omitempty"`
	LatencyMillis   int64    `json:"latency_millis,omitempty"`
	CostEstimate    float64  `json:"cost_estimate,omitempty"`
	FallbackChain   []string `json:"fallback_chain,omitempty"`
}

type usageRecorder struct {
	mu      sync.Mutex
	byModel map[string]UsageAggregate
}

func newUsageRecorder() *usageRecorder {
	return &usageRecorder{byModel: make(map[string]UsageAggregate)}
}

func (r *usageRecorder) record(usage modelUsage) {
	if r == nil || usage.Provider == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := usage.Provider + "\x00" + usage.Model
	agg := r.byModel[key]
	agg.Provider = usage.Provider
	agg.Model = usage.Model
	agg.Requests++
	agg.InputTokens += usage.InputTokens
	agg.OutputTokens += usage.OutputTokens
	agg.EmbeddingTokens += usage.EmbeddingTokens
	agg.TotalTokens += usage.InputTokens + usage.OutputTokens + usage.EmbeddingTokens
	agg.LatencyMillis += usage.LatencyMillis
	agg.CostEstimate += usage.CostEstimate
	agg.FallbackChain = append([]string(nil), usage.FallbackChain...)
	r.byModel[key] = agg
}

func (r *usageRecorder) snapshot() []UsageAggregate {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]UsageAggregate, 0, len(r.byModel))
	for _, agg := range r.byModel {
		agg.FallbackChain = append([]string(nil), agg.FallbackChain...)
		out = append(out, agg)
	}
	return out
}

type closableProvider interface {
	Close() error
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
