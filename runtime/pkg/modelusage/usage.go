// Package modelusage observes redacted accounting returned by Gateway.
// It does not select providers, retry requests, or invoke model APIs.
package modelusage

// Usage is the redacted model accounting shape returned by Gateway and
// recorded by runtime activity storage.
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
	FinishReason    string   `json:"finish_reason"`
}
