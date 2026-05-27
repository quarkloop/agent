package gatewayclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/plugin"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/runtime/pkg/runcontext"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ListModels resolves model context metadata through Gateway, the runtime's
// only model boundary.
func (p *Provider) ListModels(ctx context.Context) ([]plugin.ModelEntry, error) {
	if p == nil {
		return nil, fmt.Errorf("gateway provider is not configured")
	}
	var response gatewayv1.ListModelsResponse
	if err := p.callUnary(ctx, "list_models", &gatewayv1.ListModelsRequest{Provider: p.provider}, &response); err != nil {
		return nil, err
	}
	out := make([]plugin.ModelEntry, 0, len(response.GetModels()))
	for _, model := range response.GetModels() {
		out = append(out, plugin.ModelEntry{
			ID:            model.GetId(),
			Provider:      p.provider,
			Name:          model.GetName(),
			ContextWindow: int(model.GetContextWindow()),
			Default:       model.GetDefaultModel(),
		})
	}
	return out, nil
}

func (p *Provider) callUnary(ctx context.Context, operationName string, request, response proto.Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	client, err := natskit.Connect(callCtx, natskit.Config{
		URL:      p.cfg.URL,
		Username: p.cfg.Username,
		Password: p.cfg.Password,
		Timeout:  p.cfg.Timeout,
		Name:     "quark-runtime-gateway-client",
	})
	if err != nil {
		return fmt.Errorf("connect gateway nats: %w", err)
	}
	defer client.Close()
	operation, err := natskit.ServiceOperation("gateway", operationName)
	if err != nil {
		return err
	}
	payload, err := protojson.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal gateway request: %w", err)
	}
	envelope, err := natskit.NewRequest(natskit.NewServiceCallID(), firstNonEmpty(runcontext.SpaceID(ctx), "runtime"), natskit.ActorRuntime, json.RawMessage(payload))
	if err != nil {
		return err
	}
	envelope.SessionID = runcontext.SessionID(ctx)
	envelope.RunID = runcontext.RunID(ctx)
	result, err := client.Call(callCtx, operation, envelope)
	if err != nil {
		return fmt.Errorf("call gateway %s: %w", operationName, err)
	}
	if result.Status != natskit.StatusOK {
		if result.Error != nil {
			return fmt.Errorf("call gateway %s: %s", operationName, result.Error.Message)
		}
		return fmt.Errorf("call gateway %s failed", operationName)
	}
	if err := protojson.Unmarshal(result.Payload, response); err != nil {
		return fmt.Errorf("decode gateway %s response: %w", operationName, err)
	}
	return nil
}
