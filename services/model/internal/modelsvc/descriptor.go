package modelsvc

import (
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := modelv1.ModelService_ServiceDesc.ServiceName
	return &servicev1.ServiceDescriptor{
		Name:    "gateway",
		Type:    "gateway",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "Generate", "quark.model.v1.GenerateRequest", "quark.model.v1.GenerateResponse", "Run one non-streaming model generation request."),
			streamingRPC(serviceName, "StreamGenerate", "quark.model.v1.StreamGenerateRequest", "quark.model.v1.StreamGenerateResponse", "Run one streaming model generation request."),
			rpc(serviceName, "Embed", "quark.model.v1.EmbedRequest", "quark.model.v1.EmbedResponse", "Create embeddings through provider adapters."),
			rpc(serviceName, "Rerank", "quark.model.v1.RerankRequest", "quark.model.v1.RerankResponse", "Rerank candidate documents for a query."),
			rpc(serviceName, "CountTokens", "quark.model.v1.CountTokensRequest", "quark.model.v1.CountTokensResponse", "Count or estimate model tokens."),
			rpc(serviceName, "ListModels", "quark.model.v1.ListModelsRequest", "quark.model.v1.ListModelsResponse", "List provider models."),
			rpc(serviceName, "ProviderHealth", "quark.model.v1.ProviderHealthRequest", "quark.model.v1.ProviderHealthResponse", "Return provider adapter readiness."),
			rpc(serviceName, "UsageSummary", "quark.model.v1.UsageSummaryRequest", "quark.model.v1.UsageSummaryResponse", "Return Gateway usage aggregates."),
			rpc(serviceName, "ReloadConfig", "quark.model.v1.ReloadConfigRequest", "quark.model.v1.ReloadConfigResponse", "Reload Gateway provider policy without restarting the process."),
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
