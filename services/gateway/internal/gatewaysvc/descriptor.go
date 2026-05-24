package gatewaysvc

import (
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := "quark.gateway.v1.GatewayService"
	return &servicev1.ServiceDescriptor{
		Name:    "gateway",
		Type:    "gateway",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "Generate", "quark.gateway.v1.GenerateRequest", "quark.gateway.v1.GenerateResponse", "Run one non-streaming model generation request."),
			streamingRPC(serviceName, "StreamGenerate", "quark.gateway.v1.StreamGenerateRequest", "quark.gateway.v1.StreamGenerateResponse", "Run one streaming model generation request."),
			rpc(serviceName, "Embed", "quark.gateway.v1.EmbedRequest", "quark.gateway.v1.EmbedResponse", "Create text or supported multimodal embeddings through provider adapters."),
			rpc(serviceName, "Rerank", "quark.gateway.v1.RerankRequest", "quark.gateway.v1.RerankResponse", "Rerank candidate documents for a query."),
			rpc(serviceName, "CountTokens", "quark.gateway.v1.CountTokensRequest", "quark.gateway.v1.CountTokensResponse", "Count or estimate model tokens."),
			rpc(serviceName, "ListModels", "quark.gateway.v1.ListModelsRequest", "quark.gateway.v1.ListModelsResponse", "List provider models."),
			rpc(serviceName, "ProviderHealth", "quark.gateway.v1.ProviderHealthRequest", "quark.gateway.v1.ProviderHealthResponse", "Return provider adapter readiness."),
			rpc(serviceName, "UsageSummary", "quark.gateway.v1.UsageSummaryRequest", "quark.gateway.v1.UsageSummaryResponse", "Return Gateway usage aggregates."),
			rpc(serviceName, "ReloadConfig", "quark.gateway.v1.ReloadConfigRequest", "quark.gateway.v1.ReloadConfigResponse", "Reload Gateway provider policy without restarting the process."),
		},
		Skills: skills,
	}
}

func rpc(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return &servicev1.RpcDescriptor{
		Service:     service,
		Method:      method,
		Request:     request,
		Response:    response,
		Description: description,
	}
}

func streamingRPC(service, method, request, response, description string) *servicev1.RpcDescriptor {
	out := rpc(service, method, request, response, description)
	out.Streaming = true
	return out
}
