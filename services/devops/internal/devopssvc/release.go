package devopssvc

import (
	"context"
	"errors"
	"fmt"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"github.com/quarkloop/services/devops/internal/buildrelease"
)

func (s *Server) InitReleaseConfig(ctx context.Context, req *devopsv1.InitReleaseConfigRequest) (*devopsv1.InitReleaseConfigResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	result, err := s.releases.Init(ctx, buildrelease.InitRequest{
		WorkingDir: root,
		Overwrite:  req.GetOverwrite(),
	})
	if err != nil {
		return nil, releaseError(err)
	}
	return &devopsv1.InitReleaseConfigResponse{
		Plan:       mutationPlan("build.init_release_config", result.ConfigPath, req.GetReason(), true, "workspace.write"),
		ConfigPath: result.ConfigPath,
		Created:    result.Created,
	}, nil
}

func (s *Server) DryRunRelease(ctx context.Context, req *devopsv1.DryRunReleaseRequest) (*devopsv1.DryRunReleaseResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	result, err := s.releases.DryRun(ctx, buildrelease.DryRunRequest{
		WorkingDir:  root,
		ConfigPath:  req.GetConfigPath(),
		Version:     req.GetVersion(),
		Parallelism: int(req.GetParallelism()),
	})
	if err != nil {
		return nil, releaseError(err)
	}
	return &devopsv1.DryRunReleaseResponse{
		Version: result.Version,
		Planned: releaseArtifactsToProto(result.Planned),
	}, nil
}

func (s *Server) RunRelease(ctx context.Context, req *devopsv1.RunReleaseRequest) (*devopsv1.RunReleaseResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	result, err := s.releases.Release(ctx, buildrelease.ReleaseRequest{
		WorkingDir:  root,
		ConfigPath:  req.GetConfigPath(),
		Version:     req.GetVersion(),
		Parallelism: int(req.GetParallelism()),
		SkipTests:   req.GetSkipTests(),
	})
	if err != nil {
		return nil, releaseError(err)
	}
	return &devopsv1.RunReleaseResponse{
		Plan:       mutationPlan("build.run_release", root, req.GetReason(), true, "command.execute", "artifact.write", "release.publish"),
		Success:    result.Success,
		Message:    result.Message,
		Version:    result.Version,
		ReleaseDir: result.ReleaseDir,
		Artifacts:  releaseArtifactsToProto(result.Artifacts),
	}, nil
}

func releaseArtifactsToProto(in []buildrelease.Artifact) []*devopsv1.ReleaseArtifact {
	out := make([]*devopsv1.ReleaseArtifact, 0, len(in))
	for _, artifact := range in {
		out = append(out, &devopsv1.ReleaseArtifact{
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

func releaseError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return serviceerrors.Canceled(err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return serviceerrors.DeadlineExceeded(err.Error())
	default:
		return serviceerrors.InvalidArgument(fmt.Sprintf("%v", err))
	}
}
