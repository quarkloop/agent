package docsvc

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := "quark.document.v1.DocumentService"
	return &servicev1.ServiceDescriptor{
		Name:    "document",
		Type:    "document",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "DetectType", "quark.document.v1.DetectTypeRequest", "quark.document.v1.DetectTypeResponse", "Detect MIME type, extension, and coarse document family."),
			rpc(serviceName, "ParseBytes", "quark.document.v1.ParseBytesRequest", "quark.document.v1.ParseBytesResponse", "Parse bytes into mechanical metadata."),
			rpc(serviceName, "ExtractText", "quark.document.v1.ExtractTextRequest", "quark.document.v1.ExtractTextResponse", "Extract raw text and per-page offsets."),
			rpc(serviceName, "ExtractLayout", "quark.document.v1.ExtractLayoutRequest", "quark.document.v1.ExtractLayoutResponse", "Extract mechanical layout blocks."),
			rpc(serviceName, "GetPages", "quark.document.v1.GetPagesRequest", "quark.document.v1.GetPagesResponse", "Return pages with text, layout, tables, and images."),
			rpc(serviceName, "ExtractTables", "quark.document.v1.ExtractTablesRequest", "quark.document.v1.ExtractTablesResponse", "Extract detected table rows."),
			rpc(serviceName, "ExtractImages", "quark.document.v1.ExtractImagesRequest", "quark.document.v1.ExtractImagesResponse", "Extract image references and metadata."),
			rpc(serviceName, "RunOCR", "quark.document.v1.RunOCRRequest", "quark.document.v1.RunOCRResponse", "Run OCR when an OCR backend is available."),
		},
		Skills: skills,
	}
}

func rpc(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("document", "document_"+method, service, method, request, response, description)
}
