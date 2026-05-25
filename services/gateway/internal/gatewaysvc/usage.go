package gatewaysvc

import (
	"fmt"
	"sync"
	"time"
)

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

func (s *Server) recordUsage(usage modelUsage) {
	if s == nil || s.recorder == nil {
		return
	}
	s.recorder.record(usage)
}

func (s *Server) UsageSummarySnapshot() []UsageAggregate {
	if s == nil || s.recorder == nil {
		return nil
	}
	return s.recorder.snapshot()
}

func requestID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func elapsedMillis(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}
