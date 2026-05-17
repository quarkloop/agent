package modelclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/quarkloop/pkg/plugin"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

type Provider struct {
	address  string
	provider string
}

func New(address, provider string) *Provider {
	return &Provider{address: strings.TrimSpace(address), provider: strings.TrimSpace(provider)}
}

func (p *Provider) ChatCompletionStream(ctx context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	if p.address == "" {
		return nil, fmt.Errorf("model service address is required")
	}
	conn, err := servicekit.Dial(ctx, p.address)
	if err != nil {
		return nil, fmt.Errorf("dial model service: %w", err)
	}
	client := modelv1.NewModelServiceClient(conn)
	stream, err := client.StreamGenerate(ctx, &modelv1.StreamGenerateRequest{
		Provider: p.provider,
		Model:    requestModel(req),
		Messages: messagesToProto(req.Messages),
		Tools:    toolsToProto(req.Tools),
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	out := make(chan plugin.StreamEvent, 64)
	go func() {
		defer close(out)
		defer conn.Close()
		for {
			msg, err := stream.Recv()
			if err != nil {
				if err == context.Canceled || err == io.EOF {
					return
				}
				out <- plugin.StreamEvent{Err: err}
				return
			}
			out <- plugin.StreamEvent{
				Delta:     msg.GetDelta(),
				ToolCalls: toolCallsFromProto(msg.GetToolCalls()),
				Done:      msg.GetDone(),
			}
			if msg.GetDone() {
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

func requestModel(req *plugin.ChatRequest) string {
	if req == nil {
		return ""
	}
	return req.Model
}

var _ plugin.Provider = (*Provider)(nil)
