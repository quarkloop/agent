package citationsvc

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	return &servicev1.ServiceDescriptor{
		Name:    "citation",
		Type:    "citation",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{
			rpc("ResolveSpans", "quark.citation.v1.ResolveSpansRequest", "quark.citation.v1.ResolveSpansResponse", "Resolve selected claim text into source spans with offsets and confidence."),
			rpc("CreateCitation", "quark.citation.v1.CreateCitationRequest", "quark.citation.v1.CitationSpan", "Create one normalized citation span from source text and selected evidence."),
			rpc("VerifyGrounding", "quark.citation.v1.VerifyGroundingRequest", "quark.citation.v1.VerifyGroundingResponse", "Mechanically verify that selected claims are grounded by provided citation spans."),
			rpc("ScoreCoverage", "quark.citation.v1.ScoreCoverageRequest", "quark.citation.v1.ScoreCoverageResponse", "Score citation coverage across selected claims."),
			rpc("RenderReferences", "quark.citation.v1.RenderReferencesRequest", "quark.citation.v1.RenderReferencesResponse", "Render normalized source references for user-facing answers."),
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
}

func rpc(method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("citation", "citation_"+method, "quark.citation.v1.CitationService", method, request, response, description)
}
