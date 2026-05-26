//go:build e2e

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ModelUsage is the redacted runtime model-turn accounting record.
type ModelUsage struct {
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
	FinishReason    string   `json:"finish_reason,omitempty"`
}

// GatewayUsage is one cumulative provider/model usage aggregate reported by
// the Gateway service, including embeddings produced during agent workflows.
type GatewayUsage struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Requests        int64    `json:"requests"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	EmbeddingTokens int64    `json:"embedding_tokens"`
	TotalTokens     int64    `json:"total_tokens"`
	LatencyMillis   int64    `json:"latency_millis"`
	CostEstimate    float64  `json:"cost_estimate,omitempty"`
	FallbackChain   []string `json:"fallback_chain,omitempty"`
}

type openRouterKeyStatus struct {
	Data struct {
		LimitRemaining *float64 `json:"limit_remaining"`
		IsFreeTier     bool     `json:"is_free_tier"`
	} `json:"data"`
}

func collectUsage(t *testing.T, env *E2EEnv, trace *MessageTrace) {
	t.Helper()
	if env == nil || trace == nil || strings.TrimSpace(env.Provider) == "" {
		return
	}
	trace.ModelUsage = runtimeModelUsage(t, env, trace.SessionID)
	trace.GatewayUsage = CaptureGatewayUsage(t, env)
	for _, usage := range trace.ModelUsage {
		Logf(t, "model-request provider=%s model=%s request_id=%s prompt_tokens=%d completion_tokens=%d embedding_tokens=%d run_id=%s",
			usage.Provider, usage.Model, usage.RequestID, usage.InputTokens, usage.OutputTokens, usage.EmbeddingTokens, usage.RunID)
	}
}

// CaptureGatewayUsage reports cumulative per-test Gateway usage and applies
// external-call budgets. Each E2E deployment owns one isolated Gateway.
func CaptureGatewayUsage(t *testing.T, env *E2EEnv) []GatewayUsage {
	t.Helper()
	if env == nil || strings.TrimSpace(env.Provider) == "" {
		return nil
	}
	usages := gatewayUsageSummary(t, env)
	if len(usages) == 0 {
		t.Fatal("Gateway returned no usage records after a real-model request")
	}
	var requests, tokens int64
	for _, usage := range usages {
		requests += usage.Requests
		tokens += usage.TotalTokens
		Logf(t, "model-usage provider=%s model=%s requests=%d prompt_tokens=%d completion_tokens=%d embedding_tokens=%d total_tokens=%d cost_estimate=%g",
			usage.Provider, usage.Model, usage.Requests, usage.InputTokens, usage.OutputTokens, usage.EmbeddingTokens, usage.TotalTokens, usage.CostEstimate)
	}
	budgetRequests := int64Env("QUARK_E2E_MAX_PROVIDER_REQUESTS", 100)
	budgetTokens := int64Env("QUARK_E2E_MAX_TOTAL_TOKENS", 250000)
	if err := validateGatewayUsageBudget(requests, tokens, budgetRequests, budgetTokens); err != nil {
		t.Fatal(err)
	}
	return usages
}

func preflightGateway(t *testing.T, env *E2EEnv) {
	t.Helper()
	verifyOpenRouterCreditPrerequisite(t, env)
	var health gatewayv1.ProviderHealthResponse
	callGateway(t, env, "provider_health", &gatewayv1.ProviderHealthRequest{Provider: env.Provider}, &health)
	if !health.GetHealthy() {
		t.Fatalf("Gateway provider %q is not ready: %s", health.GetProvider(), health.GetStatus())
	}
	var generated gatewayv1.GenerateResponse
	callGateway(t, env, "generate", &gatewayv1.GenerateRequest{
		Provider: env.Provider,
		Model:    env.Model,
		Messages: []*gatewayv1.ModelMessage{{
			Role: "user",
			Content: []*gatewayv1.ContentPart{{
				Kind: gatewayv1.ContentKind_CONTENT_KIND_TEXT,
				Text: "Reply with OK.",
			}},
		}},
		Options: map[string]string{"max_output_tokens": "4"},
	}, &generated)
	usage := generated.GetUsage()
	if strings.TrimSpace(generated.GetText()) == "" || usage.GetRequestId() == "" {
		t.Fatalf("Gateway provider preflight returned incomplete generation metadata: %+v", &generated)
	}
	Logf(t, "provider-preflight provider=%s model=%s request_id=%s prompt_tokens=%d completion_tokens=%d", usage.GetProvider(), usage.GetModel(), usage.GetRequestId(), usage.GetInputTokens(), usage.GetOutputTokens())
	CaptureGatewayUsage(t, env)
}

func callGateway(t *testing.T, env *E2EEnv, function string, req proto.Message, resp proto.Message) {
	t.Helper()
	conn, err := natskit.Connect(context.Background(), natskit.Config{
		URL: env.NATS.ClientURL, Username: natshub.DefaultControlUser,
		Password: natshub.DefaultControlPassword, Name: "quark-e2e-gateway-preflight",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("connect NATS for Gateway preflight: %v", err)
	}
	defer conn.Close()
	payload, err := protojson.Marshal(req)
	if err != nil {
		t.Fatalf("marshal Gateway %s request: %v", function, err)
	}
	operation, err := natskit.ServiceOperation("gateway", function)
	if err != nil {
		t.Fatalf("Gateway %s operation: %v", function, err)
	}
	request, err := natskit.NewRequest(natskit.NewServiceCallID(), env.Space, natskit.ActorRuntime, payload)
	if err != nil {
		t.Fatalf("Gateway %s envelope: %v", function, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	envelope, err := conn.Call(ctx, operation, request)
	if err != nil {
		t.Fatalf("Gateway %s request: %v", function, err)
	}
	if envelope.Status != natskit.StatusOK {
		t.Fatalf("Gateway %s rejected preflight: %+v", function, envelope.Error)
	}
	if err := protojson.Unmarshal(envelope.Payload, resp); err != nil {
		t.Fatalf("decode Gateway %s response: %v", function, err)
	}
}

func verifyOpenRouterCreditPrerequisite(t *testing.T, env *E2EEnv) {
	t.Helper()
	if env.Provider != "openrouter" {
		return
	}
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		t.Fatal("OPENROUTER_API_KEY is required for OpenRouter preflight")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/key", nil)
	if err != nil {
		t.Fatalf("build OpenRouter credit preflight request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request OpenRouter key status: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("OpenRouter key status HTTP %d", response.StatusCode)
	}
	status, err := decodeOpenRouterKeyStatus(response.Body)
	if err != nil {
		t.Fatalf("decode OpenRouter key status: %v", err)
	}
	if err := validateOpenRouterKeyStatus(status); err != nil {
		t.Fatal(err)
	}
	remaining := "unlimited"
	if status.Data.LimitRemaining != nil {
		remaining = fmt.Sprintf("%g", *status.Data.LimitRemaining)
	}
	Logf(t, "provider-credit-preflight provider=openrouter free_tier=%t limit_remaining=%s", status.Data.IsFreeTier, remaining)
}

func validateGatewayUsageBudget(requests, tokens, budgetRequests, budgetTokens int64) error {
	if requests > budgetRequests {
		return fmt.Errorf("Gateway request budget exceeded: requests=%d budget=%d", requests, budgetRequests)
	}
	if tokens > budgetTokens {
		return fmt.Errorf("Gateway token budget exceeded: tokens=%d budget=%d", tokens, budgetTokens)
	}
	return nil
}

func decodeOpenRouterKeyStatus(r io.Reader) (openRouterKeyStatus, error) {
	var status openRouterKeyStatus
	if err := json.NewDecoder(r).Decode(&status); err != nil {
		return openRouterKeyStatus{}, err
	}
	return status, nil
}

func validateOpenRouterKeyStatus(status openRouterKeyStatus) error {
	if status.Data.LimitRemaining != nil && *status.Data.LimitRemaining < 0 {
		return fmt.Errorf("OpenRouter key credit remaining is negative: %g", *status.Data.LimitRemaining)
	}
	return nil
}

func runtimeModelUsage(t *testing.T, env *E2EEnv, sessionID string) []ModelUsage {
	t.Helper()
	credential := issueSpaceScopedCredential(t, env.NATS, clientcontract.SubjectSpaceCredential, env.Space)
	conn := connectNATSCredential(t, credential)
	defer conn.Close()
	response := requestNATSPayload[clientcontract.RuntimeActivityListResponse](t, conn, clientcontract.SubjectRuntimeActivityList, env.Space, clientcontract.RuntimeActivityListRequest{SpaceID: env.Space, Limit: 1000})
	out := make([]ModelUsage, 0)
	for _, record := range response.Records {
		if record.Type != "model.usage" || record.SessionID != sessionID {
			continue
		}
		var usage ModelUsage
		if err := json.Unmarshal(record.Data, &usage); err != nil {
			t.Fatalf("decode runtime model usage activity: %v", err)
		}
		usage.FallbackChain = append([]string(nil), usage.FallbackChain...)
		out = append(out, usage)
	}
	return out
}

func gatewayUsageSummary(t *testing.T, env *E2EEnv) []GatewayUsage {
	t.Helper()
	conn, err := natskit.Connect(context.Background(), natskit.Config{
		URL: env.NATS.ClientURL, Username: natshub.DefaultControlUser,
		Password: natshub.DefaultControlPassword, Name: "quark-e2e-gateway-usage",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("connect NATS for Gateway usage: %v", err)
	}
	defer conn.Close()
	payload, err := protojson.Marshal(&gatewayv1.UsageSummaryRequest{})
	if err != nil {
		t.Fatalf("marshal Gateway usage request: %v", err)
	}
	operation, err := natskit.ServiceOperation("gateway", "usage_summary")
	if err != nil {
		t.Fatalf("Gateway usage operation: %v", err)
	}
	request, err := natskit.NewRequest(natskit.NewServiceCallID(), env.Space, natskit.ActorRuntime, payload)
	if err != nil {
		t.Fatalf("Gateway usage request envelope: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	envelope, err := conn.Call(ctx, operation, request)
	if err != nil {
		t.Fatalf("Gateway usage request: %v", err)
	}
	if envelope.Status != natskit.StatusOK {
		t.Fatalf("Gateway usage response failed: %+v", envelope.Error)
	}
	var response gatewayv1.UsageSummaryResponse
	if err := protojson.Unmarshal(envelope.Payload, &response); err != nil {
		t.Fatalf("decode Gateway usage response: %v", err)
	}
	out := make([]GatewayUsage, 0, len(response.GetUsage()))
	for _, usage := range response.GetUsage() {
		out = append(out, GatewayUsage{
			Provider: usage.GetProvider(), Model: usage.GetModel(), Requests: usage.GetRequests(),
			InputTokens: usage.GetInputTokens(), OutputTokens: usage.GetOutputTokens(),
			EmbeddingTokens: usage.GetEmbeddingTokens(), TotalTokens: usage.GetTotalTokens(),
			LatencyMillis: usage.GetLatencyMillis(), CostEstimate: usage.GetCostEstimate(),
			FallbackChain: append([]string(nil), usage.GetFallbackChain()...),
		})
	}
	return out
}

func int64Env(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func formatUsageRecords(usages []GatewayUsage) string {
	parts := make([]string, 0, len(usages))
	for _, usage := range usages {
		parts = append(parts, fmt.Sprintf("%s/%s:%d", usage.Provider, usage.Model, usage.TotalTokens))
	}
	return strings.Join(parts, ",")
}
