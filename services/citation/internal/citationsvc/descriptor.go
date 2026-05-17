package citationsvc

import (
	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	return &servicev1.ServiceDescriptor{
		Name:    "citation",
		Type:    "citation",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			{Service: citationv1.CitationService_ServiceDesc.ServiceName, Method: "ResolveSpans", Request: "quark.citation.v1.ResolveSpansRequest", Response: "quark.citation.v1.ResolveSpansResponse", Description: "Resolve selected claim text into source spans with offsets and confidence."},
			{Service: citationv1.CitationService_ServiceDesc.ServiceName, Method: "CreateCitation", Request: "quark.citation.v1.CreateCitationRequest", Response: "quark.citation.v1.CitationSpan", Description: "Create one normalized citation span from source text and selected evidence."},
			{Service: citationv1.CitationService_ServiceDesc.ServiceName, Method: "VerifyGrounding", Request: "quark.citation.v1.VerifyGroundingRequest", Response: "quark.citation.v1.VerifyGroundingResponse", Description: "Mechanically verify that selected claims are grounded by provided citation spans."},
			{Service: citationv1.CitationService_ServiceDesc.ServiceName, Method: "ScoreCoverage", Request: "quark.citation.v1.ScoreCoverageRequest", Response: "quark.citation.v1.ScoreCoverageResponse", Description: "Score citation coverage across selected claims."},
			{Service: citationv1.CitationService_ServiceDesc.ServiceName, Method: "RenderReferences", Request: "quark.citation.v1.RenderReferencesRequest", Response: "quark.citation.v1.RenderReferencesResponse", Description: "Render normalized source references for user-facing answers."},
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
}
