package modelservice

// Usage is the redacted model accounting shape emitted by the model boundary.
// It intentionally excludes prompts, tool arguments, raw response text, and
// provider credentials.
type Usage struct {
	SessionID       string   `json:"session_id,omitempty"`
	RunID           string   `json:"run_id,omitempty"`
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	ReasoningTokens int64    `json:"reasoning_tokens,omitempty"`
	CachedTokens    int64    `json:"cached_tokens,omitempty"`
	EmbeddingTokens int64    `json:"embedding_tokens,omitempty"`
	LatencyMillis   int64    `json:"latency_millis"`
	CostEstimate    float64  `json:"cost_estimate,omitempty"`
	FallbackChain   []string `json:"fallback_chain,omitempty"`
	RequestID       string   `json:"request_id,omitempty"`
	FailureCategory string   `json:"failure_category,omitempty"`
	FailureResetAt  string   `json:"failure_reset_at,omitempty"`
	FinishReason    string   `json:"finish_reason"`
}

func cloneUsage(usage Usage) Usage {
	usage.FallbackChain = append([]string(nil), usage.FallbackChain...)
	return usage
}
