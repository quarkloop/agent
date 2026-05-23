package gatewayclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/plugin"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	EnvNATSURL      = "QUARK_NATS_URL"
	EnvNATSUser     = "QUARK_NATS_USER"
	EnvNATSPassword = "QUARK_NATS_PASSWORD"

	DefaultURL     = "nats://127.0.0.1:4222"
	DefaultUser    = "quark-runtime"
	DefaultTimeout = 30 * time.Second
)

type Config struct {
	URL      string
	Username string
	Password string
	Timeout  time.Duration
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
		URL:      firstNonEmpty(os.Getenv(EnvNATSURL), DefaultURL),
		Username: firstNonEmpty(os.Getenv(EnvNATSUser), DefaultUser),
		Password: os.Getenv(EnvNATSPassword),
		Timeout:  DefaultTimeout,
	}
}

func (p *Provider) ChatCompletionStream(ctx context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	if p == nil {
		return nil, fmt.Errorf("gateway provider is not configured")
	}
	conn, err := natsgo.Connect(p.cfg.URL, natsgo.UserInfo(p.cfg.Username, p.cfg.Password), natsgo.Timeout(p.cfg.Timeout), natsgo.Name("quark-runtime-gateway-client"))
	if err != nil {
		return nil, fmt.Errorf("connect gateway nats: %w", err)
	}
	subject, err := servicefunction.Subject("gateway", servicefunction.DefaultVersion, "stream_generate")
	if err != nil {
		conn.Close()
		return nil, err
	}
	payload, err := protojson.Marshal(&modelv1.StreamGenerateRequest{
		Provider: p.provider,
		Model:    requestModel(req),
		Messages: messagesToProto(req.Messages),
		Tools:    toolsToProto(req.Tools),
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("marshal gateway request: %w", err)
	}
	envelope, err := servicefunction.NewRequest(newCallID(), firstNonEmpty(modelservice.SpaceID(ctx), "runtime"), servicefunction.ActorRuntime, servicefunction.Descriptor{
		Version:       servicefunction.DescriptorVersion,
		Service:       "gateway",
		Function:      "stream_generate",
		Subject:       subject,
		InputSchema:   json.RawMessage(`{"type":"object"}`),
		OutputSchema:  json.RawMessage(`{"type":"object"}`),
		Risk:          servicefunction.RiskRead,
		TimeoutMillis: int64(p.cfg.Timeout / time.Millisecond),
	}, payload)
	if err != nil {
		conn.Close()
		return nil, err
	}
	envelope.SessionID = modelservice.SessionID(ctx)
	envelope.RunID = modelservice.RunID(ctx)
	data, err := json.Marshal(envelope)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("encode gateway request: %w", err)
	}
	inbox := natsgo.NewInbox()
	sub, err := conn.SubscribeSync(inbox)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("subscribe gateway reply: %w", err)
	}
	if err := conn.PublishRequest(subject, inbox, data); err != nil {
		sub.Unsubscribe()
		conn.Close()
		return nil, fmt.Errorf("publish gateway request: %w", err)
	}
	out := make(chan plugin.StreamEvent, 64)
	go func() {
		defer close(out)
		defer sub.Unsubscribe()
		defer conn.Close()
		for {
			msg, err := sub.NextMsgWithContext(ctx)
			if err != nil {
				out <- plugin.StreamEvent{Err: err}
				return
			}
			var envelope servicefunction.ResponseEnvelope
			if err := json.Unmarshal(msg.Data, &envelope); err != nil {
				out <- plugin.StreamEvent{Err: err}
				return
			}
			if envelope.Status == servicefunction.StatusError {
				out <- plugin.StreamEvent{Err: fmt.Errorf("%s", envelope.Error.Message)}
				return
			}
			var chunk modelv1.StreamGenerateResponse
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
		}
	}()
	return out, nil
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

func messagesToProto(messages []plugin.Message) []*modelv1.ModelMessage {
	out := make([]*modelv1.ModelMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, &modelv1.ModelMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  toolCallsToProto(msg.ToolCalls),
			ToolCallId: msg.ToolCallID,
		})
	}
	return out
}

func toolsToProto(tools []plugin.ToolSchema) []*modelv1.ToolSchema {
	out := make([]*modelv1.ToolSchema, 0, len(tools))
	for _, tool := range tools {
		params, _ := json.Marshal(tool.Parameters)
		out = append(out, &modelv1.ToolSchema{
			Name:           tool.Name,
			Description:    tool.Description,
			ParametersJson: string(params),
		})
	}
	return out
}

func toolCallsToProto(calls []plugin.ToolCall) []*modelv1.ToolCall {
	out := make([]*modelv1.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, &modelv1.ToolCall{
			Index:         int32(call.Index),
			Id:            call.ID,
			Type:          call.Type,
			Name:          call.Function.Name,
			ArgumentsJson: call.Function.Arguments,
		})
	}
	return out
}

func toolCallsFromProto(calls []*modelv1.ToolCall) []plugin.ToolCall {
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

func streamUsageFromProto(usage *modelv1.ModelUsage, envelopeUsage *servicefunction.Usage) *plugin.StreamUsage {
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

func newCallID() string {
	return fmt.Sprintf("gateway-%d", time.Now().UnixNano())
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
