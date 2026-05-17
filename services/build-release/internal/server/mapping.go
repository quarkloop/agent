package server

import (
	buildreleasev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/buildrelease/v1"
	"github.com/quarkloop/services/build-release/pkg/buildrelease"
)

func releaseRequestFromProto(req *buildreleasev1.ReleaseRequest) buildrelease.ReleaseRequest {
	if req == nil {
		return buildrelease.ReleaseRequest{}
	}
	return buildrelease.ReleaseRequest{
		WorkingDir:  req.GetWorkingDir(),
		ConfigPath:  req.GetConfigPath(),
		Version:     req.GetVersion(),
		Parallelism: int(req.GetParallelism()),
		SkipTests:   req.GetSkipTests(),
	}
}

func releaseResponseToProto(result *buildrelease.ReleaseResult) *buildreleasev1.ReleaseResponse {
	if result == nil {
		return &buildreleasev1.ReleaseResponse{}
	}
	return &buildreleasev1.ReleaseResponse{
		Success:    result.Success,
		Message:    result.Message,
		Version:    result.Version,
		ReleaseDir: result.ReleaseDir,
		Artifacts:  artifactsToProto(result.Artifacts),
	}
}

func dryRunRequestFromProto(req *buildreleasev1.DryRunRequest) buildrelease.DryRunRequest {
	if req == nil {
		return buildrelease.DryRunRequest{}
	}
	return buildrelease.DryRunRequest{
		WorkingDir:  req.GetWorkingDir(),
		ConfigPath:  req.GetConfigPath(),
		Version:     req.GetVersion(),
		Parallelism: int(req.GetParallelism()),
	}
}

func dryRunResponseToProto(result *buildrelease.DryRunResult) *buildreleasev1.DryRunResponse {
	if result == nil {
		return &buildreleasev1.DryRunResponse{}
	}
	return &buildreleasev1.DryRunResponse{
		Version: result.Version,
		Planned: artifactsToProto(result.Planned),
	}
}

func initRequestFromProto(req *buildreleasev1.InitRequest) buildrelease.InitRequest {
	if req == nil {
		return buildrelease.InitRequest{}
	}
	return buildrelease.InitRequest{
		WorkingDir: req.GetWorkingDir(),
		Overwrite:  req.GetOverwrite(),
	}
}

func initResponseToProto(result *buildrelease.InitResult) *buildreleasev1.InitResponse {
	if result == nil {
		return &buildreleasev1.InitResponse{}
	}
	return &buildreleasev1.InitResponse{
		ConfigPath: result.ConfigPath,
		Created:    result.Created,
	}
}

func artifactsToProto(in []buildrelease.Artifact) []*buildreleasev1.Artifact {
	out := make([]*buildreleasev1.Artifact, 0, len(in))
	for _, artifact := range in {
		out = append(out, &buildreleasev1.Artifact{
			BuildName:      artifact.BuildName,
			Os:             artifact.Target.OS,
			Arch:           artifact.Target.Arch,
			Arm:            artifact.Target.ARM,
			Filename:       artifact.Filename,
			ArchiveName:    artifact.ArchiveName,
			Checksum:       artifact.Checksum,
			Size:           artifact.Size,
			DurationMillis: artifact.Duration.Milliseconds(),
			Error:          artifact.Error,
		})
	}
	return out
}
