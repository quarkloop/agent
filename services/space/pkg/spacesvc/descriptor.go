package spacesvc

import (
	"github.com/quarkloop/pkg/natskit"
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
			rpc("CreateSpace", "quark.space.v1.CreateSpaceRequest", "quark.space.v1.Space", "Create a space and persist its initial configuration."),
			rpc("UpdateConfig", "quark.space.v1.UpdateConfigRequest", "quark.space.v1.Space", "Replace the authoritative configuration for a space."),
			rpc("GetSpace", "quark.space.v1.GetSpaceRequest", "quark.space.v1.Space", "Return persisted space identity metadata."),
			rpc("ListSpaces", "google.protobuf.Empty", "quark.space.v1.ListSpacesResponse", "List registered spaces."),
			rpc("DeleteSpace", "quark.space.v1.DeleteSpaceRequest", "google.protobuf.Empty", "Delete a space and its service-owned data."),
			rpc("GetConfig", "quark.space.v1.GetConfigRequest", "quark.space.v1.ConfigResponse", "Return the authoritative space configuration."),
			rpc("GetAgentEnvironment", "quark.space.v1.GetAgentEnvironmentRequest", "quark.space.v1.AgentEnvironmentResponse", "Resolve model environment entries for runtime launch."),
			rpc("GetSpacePaths", "quark.space.v1.GetSpacePathsRequest", "quark.space.v1.SpacePaths", "Return derived storage paths for a space."),
			rpc("Doctor", "quark.space.v1.DoctorRequest", "quark.space.v1.DoctorResponse", "Validate the persisted space configuration."),
		},
		Skills: skills,
	}
}

func rpc(method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("space", "space_"+method, "quark.space.v1.SpaceService", method, request, response, description)
}
