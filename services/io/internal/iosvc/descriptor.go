package iosvc

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := "quark.io.v1.IOService"
	return &servicev1.ServiceDescriptor{
		Name:    "io",
		Type:    "io",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "Read", "quark.io.v1.ReadRequest", "quark.io.v1.ReadResponse", "Read a text file, optionally with a line range."),
			rpc(serviceName, "List", "quark.io.v1.ListRequest", "quark.io.v1.ListResponse", "List directory entries with optional recursion and hashes."),
			rpc(serviceName, "Stat", "quark.io.v1.StatRequest", "quark.io.v1.StatResponse", "Return file metadata and optional sha256."),
			rpc(serviceName, "Write", "quark.io.v1.WriteRequest", "quark.io.v1.WriteResponse", "Overwrite a file after explicit approval."),
			rpc(serviceName, "Append", "quark.io.v1.AppendRequest", "quark.io.v1.AppendResponse", "Append to a file after explicit approval."),
			rpc(serviceName, "Replace", "quark.io.v1.ReplaceRequest", "quark.io.v1.ReplaceResponse", "Replace text in a file after explicit approval."),
			rpc(serviceName, "Remove", "quark.io.v1.RemoveRequest", "quark.io.v1.RemoveResponse", "Remove a file or directory after explicit approval."),
			rpc(serviceName, "ReadMedia", "quark.io.v1.ReadMediaRequest", "quark.io.v1.ReadMediaResponse", "Read bounded media bytes and source metadata for runtime-managed references."),
			rpc(serviceName, "ExtractPdf", "quark.io.v1.ExtractPdfRequest", "quark.io.v1.ExtractPdfResponse", "Extract PDF text with pdftotext."),
			rpc(serviceName, "Execute", "quark.io.v1.ExecuteRequest", "quark.io.v1.ExecuteResponse", "Execute a shell command via bash -c after explicit approval."),
			rpc(serviceName, "SearchWeb", "quark.io.v1.SearchWebRequest", "quark.io.v1.SearchWebResponse", "Search the web using Brave or SerpAPI."),
			rpc(serviceName, "Fetch", "quark.io.v1.FetchRequest", "quark.io.v1.FetchResponse", "Fetch a URL over HTTP or HTTPS with size and timeout limits."),
		},
		Skills: skills,
	}
}

func rpc(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("io", "io_"+method, service, method, request, response, description)
}
