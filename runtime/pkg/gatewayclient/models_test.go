package gatewayclient

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/pkg/natskit"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

type modelCatalogFixture struct{}

func (modelCatalogFixture) ListModels(_ context.Context, request *gatewayv1.ListModelsRequest) (*gatewayv1.ListModelsResponse, error) {
	return &gatewayv1.ListModelsResponse{Models: []*gatewayv1.ModelInfo{{
		Id:            request.GetProvider() + "/chat",
		Provider:      request.GetProvider(),
		Name:          "Gateway Chat",
		ContextWindow: 128000,
		DefaultModel:  true,
	}}}, nil
}

func TestProviderListsModelContextMetadataThroughGateway(t *testing.T) {
	ns := startProviderTestNATS(t)
	host, err := natskit.StartRPCService(context.Background(), natskit.Config{
		URL: ns.ClientURL(), Username: "quark-control", Password: "secret",
		Name: "gateway-model-catalog-test", Timeout: time.Second,
	}, natskit.Binding{
		Descriptor: &servicev1.ServiceDescriptor{Name: "gateway", Rpcs: []*servicev1.RpcDescriptor{
			natskit.MustServiceRPC("gateway", "gateway_ListModels", "quark.gateway.v1.GatewayService", "ListModels", "quark.gateway.v1.ListModelsRequest", "quark.gateway.v1.ListModelsResponse", "List models."),
		}},
		Services: []natskit.RPCService{{Service: "quark.gateway.v1.GatewayService", Implementation: modelCatalogFixture{}}},
	})
	if err != nil {
		t.Fatalf("start gateway catalog fixture: %v", err)
	}
	t.Cleanup(host.Close)

	provider := New(Config{URL: ns.ClientURL(), Username: "quark-control", Password: "secret", Timeout: time.Second}, "openrouter")
	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "openrouter/chat" || models[0].Provider != "openrouter" ||
		models[0].ContextWindow != 128000 || !models[0].Default {
		t.Fatalf("models = %+v", models)
	}
}
