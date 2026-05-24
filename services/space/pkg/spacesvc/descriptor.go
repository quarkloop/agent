package spacesvc

import (
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	return &servicev1.ServiceDescriptor{
		Name:    "space",
		Type:    "space",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			{Service: "quark.space.v1.SpaceService", Method: "CreateSpace", Request: "quark.space.v1.CreateSpaceRequest", Response: "quark.space.v1.Space", Description: "Create a space and persist its initial Quarkfile."},
			{Service: "quark.space.v1.SpaceService", Method: "UpdateQuarkfile", Request: "quark.space.v1.UpdateQuarkfileRequest", Response: "quark.space.v1.Space", Description: "Replace the latest Quarkfile for a space."},
			{Service: "quark.space.v1.SpaceService", Method: "GetSpace", Request: "quark.space.v1.GetSpaceRequest", Response: "quark.space.v1.Space", Description: "Return persisted space metadata."},
			{Service: "quark.space.v1.SpaceService", Method: "ListSpaces", Request: "google.protobuf.Empty", Response: "quark.space.v1.ListSpacesResponse", Description: "List registered spaces."},
			{Service: "quark.space.v1.SpaceService", Method: "DeleteSpace", Request: "quark.space.v1.DeleteSpaceRequest", Response: "google.protobuf.Empty", Description: "Delete a space and its service-owned data."},
			{Service: "quark.space.v1.SpaceService", Method: "GetQuarkfile", Request: "quark.space.v1.GetQuarkfileRequest", Response: "quark.space.v1.QuarkfileResponse", Description: "Return the authoritative Quarkfile bytes."},
			{Service: "quark.space.v1.SpaceService", Method: "GetAgentEnvironment", Request: "quark.space.v1.GetAgentEnvironmentRequest", Response: "quark.space.v1.AgentEnvironmentResponse", Description: "Resolve model environment entries for runtime launch."},
			{Service: "quark.space.v1.SpaceService", Method: "GetSpacePaths", Request: "quark.space.v1.GetSpacePathsRequest", Response: "quark.space.v1.SpacePaths", Description: "Return derived storage paths for a space."},
			{Service: "quark.space.v1.SpaceService", Method: "Doctor", Request: "quark.space.v1.DoctorRequest", Response: "quark.space.v1.DoctorResponse", Description: "Run Quarkfile and installed-plugin diagnostics."},
		},
		Skills: skills,
	}
}
