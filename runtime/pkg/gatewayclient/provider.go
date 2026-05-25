package gatewayclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/plugin"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	EnvNATSURL         = "QUARK_NATS_URL"
	EnvNATSUser        = "QUARK_NATS_USER"
	EnvNATSPassword    = "QUARK_NATS_PASSWORD"
	EnvGatewayTimeout  = "QUARK_GATEWAY_REQUEST_TIMEOUT"
	EnvMaxOutputTokens = "QUARK_GATEWAY_MAX_OUTPUT_TOKENS"

	DefaultURL     = "nats://127.0.0.1:4222"
	DefaultUser    = "quark-runtime"
	DefaultTimeout = 30 * time.Second
)

type Config struct {
	URL             string
	Username        string
	Password        string
	Timeout         time.Duration
	MaxOutputTokens int
}

type Provider struct {
	cfg      Config
	provider string
}

func New(cfg Config, provider string) *Provider {
	return &Provider{cfg: normalizeConfig(cfg), provider: strings.TrimSpace(provider)}
}

func ConfigFromEnv() Config {
	return Config{
		URL:             firstNonEmpty(os.Getenv(EnvNATSURL), DefaultURL),
		Username:        firstNonEmpty(os.Getenv(EnvNATSUser), DefaultUser),
		Password:        os.Getenv(EnvNATSPassword),
		Timeout:         positiveDurationFromEnv(EnvGatewayTimeout, DefaultTimeout),
		MaxOutputTokens: positiveIntFromEnv(EnvMaxOutputTokens),
	}
}

func (p *Provider) ChatCompletionStream(ctx context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	if p == nil {
		return nil, fmt.Errorf("gateway provider is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	streamCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	client, err := natskit.Connect(streamCtx, natskit.Config{
		URL:      p.cfg.URL,
		Username: p.cfg.Username,
		Password: p.cfg.Password,
		Timeout:  p.cfg.Timeout,
		Name:     "quark-runtime-gateway-client",
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect gateway nats: %w", err)
	}
	operation, err := natskit.ServiceOperation("gateway", "stream_generate")
	if err != nil {
		cancel()
		client.Close()
		return nil, err
	}
	payload, err := protojson.Marshal(&gatewayv1.StreamGenerateRequest{
		Provider: p.provider,
		Model:    requestModel(req),
		Messages: messagesToProto(req.Messages),
		Tools:    toolsToProto(req.Tools),
		Options:  requestOptions(p.cfg),
	})
	if err != nil {
		cancel()
		client.Close()
		return nil, fmt.Errorf("marshal gateway request: %w", err)
	}
	envelope, err := natskit.NewRequest(natskit.NewServiceCallID(), firstNonEmpty(modelservice.SpaceID(ctx), "runtime"), natskit.ActorRuntime, payload)
	if err != nil {
		cancel()
		client.Close()
		return nil, err
	}
	envelope.SessionID = modelservice.SessionID(ctx)
	envelope.RunID = modelservice.RunID(ctx)
	stream, err := client.OpenServiceStream(streamCtx, operation, envelope)
	if err != nil {
		cancel()
		client.Close()
		return nil, fmt.Errorf("open gateway stream: %w", err)
	}
	out := make(chan plugin.StreamEvent, 64)
	go func() {
		defer close(out)
		defer cancel()
		defer stream.Close()
		defer client.Close()
		for {
			data, err := stream.Next(streamCtx)
			if err != nil {
				out <- plugin.StreamEvent{Err: fmt.Errorf("gateway stream wait: %w", err)}
				return
			}
			envelope, err := natskit.DecodeServiceResponse(data)
			if err != nil {
				out <- plugin.StreamEvent{Err: err}
				return
			}
			if envelope.Status == natskit.StatusError {
				out <- plugin.StreamEvent{Err: fmt.Errorf("%s", envelope.Error.Message)}
				return
			}
			var chunk gatewayv1.StreamGenerateResponse
			if err := protojson.Unmarshal(envelope.Payload, &chunk); err != nil {
				out <- plugin.StreamEvent{Err: err}
				return
			}
			event := plugin.StreamEvent{
				Delta:     chunk.GetDelta(),
				ToolCalls: toolCallsFromProto(chunk.GetToolCalls()),
				Done:      chunk.GetDone(),
				Usage:     streamUsageFromProto(chunk.GetUsage(), envelope.Usage),
			}
			out <- event
			if event.Done {
				return
			}
			if envelope.Final {
				out <- plugin.StreamEvent{Err: fmt.Errorf("gateway stream completed without a done response")}
				return
			}
		}
	}()
	return out, nil
}

func requestOptions(cfg Config) map[string]string {
	if cfg.MaxOutputTokens <= 0 {
		return nil
	}
	return map[string]string{"max_output_tokens": strconv.Itoa(cfg.MaxOutputTokens)}
}

func positiveIntFromEnv(key string) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func positiveDurationFromEnv(key string, fallback time.Duration) time.Duration {
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

func (p *Provider) ParseToolCalls(content string) ([]plugin.ToolCall, string) {
	re := regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	matches := re.FindAllStringSubmatch(content, -1)
	calls := make([]plugin.ToolCall, 0, len(matches))
	for i, match := range matches {
		if len(match) < 2 {
			continue
		}
		var payload struct {
			Name      string `json:"name"`
			Arguments any    `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(match[1]), &payload); err != nil {
			continue
		}
		args, _ := json.Marshal(payload.Arguments)
		calls = append(calls, plugin.ToolCall{
			Index: i,
			ID:    fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i),
			Type:  "function",
			Function: plugin.ToolCallFunction{
				Name:      payload.Name,
				Arguments: string(args),
			},
		})
	}
	return calls, strings.TrimSpace(re.ReplaceAllString(content, ""))
}

func messagesToProto(messages []plugin.Message) []*gatewayv1.ModelMessage {
	out := make([]*gatewayv1.ModelMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, &gatewayv1.ModelMessage{
			Role:       msg.Role,
			Content:    []*gatewayv1.ContentPart{{Kind: gatewayv1.ContentKind_CONTENT_KIND_TEXT, Text: msg.Content}},
			ToolCalls:  toolCallsToProto(msg.ToolCalls),
			ToolCallId: msg.ToolCallID,
		})
	}
	return out
}

func toolsToProto(tools []plugin.ToolSchema) []*gatewayv1.ToolSchema {
	out := make([]*gatewayv1.ToolSchema, 0, len(tools))
	for _, tool := range tools {
		params, _ := json.Marshal(tool.Parameters)
		out = append(out, &gatewayv1.ToolSchema{
			Name:           tool.Name,
			Description:    tool.Description,
			ParametersJson: string(params),
		})
	}
	return out
}

func toolCallsToProto(calls []plugin.ToolCall) []*gatewayv1.ToolCall {
	out := make([]*gatewayv1.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, &gatewayv1.ToolCall{
			Index:         int32(call.Index),
			Id:            call.ID,
			Type:          call.Type,
			Name:          call.Function.Name,
			ArgumentsJson: call.Function.Arguments,
		})
	}
	return out
}

func toolCallsFromProto(calls []*gatewayv1.ToolCall) []plugin.ToolCall {
	out := make([]plugin.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, plugin.ToolCall{
			Index: int(call.GetIndex()),
			ID:    call.GetId(),
			Type:  call.GetType(),
			Function: plugin.ToolCallFunction{
				Name:      call.GetName(),
				Arguments: call.GetArgumentsJson(),
			},
		})
	}
	return out
}

func streamUsageFromProto(usage *gatewayv1.ModelUsage, envelopeUsage *natskit.Usage) *plugin.StreamUsage {
	if usage == nil && envelopeUsage == nil {
		return nil
	}
	out := &plugin.StreamUsage{}
	if usage != nil {
		out.Provider = usage.GetProvider()
		out.Model = usage.GetModel()
		out.InputTokens = usage.GetInputTokens()
		out.OutputTokens = usage.GetOutputTokens()
		out.EmbeddingTokens = usage.GetEmbeddingTokens()
		out.LatencyMillis = usage.GetLatencyMillis()
		out.CostEstimate = usage.GetCostEstimate()
		out.FallbackChain = append([]string(nil), usage.GetFallbackChain()...)
		out.RequestID = usage.GetRequestId()
		out.FinishReason = usage.GetFinishReason()
	}
	if envelopeUsage != nil {
		out.Provider = firstNonEmpty(out.Provider, envelopeUsage.Provider)
		out.Model = firstNonEmpty(out.Model, envelopeUsage.Model)
		out.RequestID = firstNonEmpty(out.RequestID, envelopeUsage.RequestID)
		if out.InputTokens == 0 {
			out.InputTokens = envelopeUsage.InputTokens
		}
		if out.OutputTokens == 0 {
			out.OutputTokens = envelopeUsage.OutputTokens
		}
	}
	return out
}

func requestModel(req *plugin.ChatRequest) string {
	if req == nil {
		return ""
	}
	return req.Model
}

func normalizeConfig(cfg Config) Config {
	cfg.URL = firstNonEmpty(cfg.URL, DefaultURL)
	cfg.Username = firstNonEmpty(cfg.Username, DefaultUser)
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var _ plugin.Provider = (*Provider)(nil)
