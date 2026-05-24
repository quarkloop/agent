package server

import (
	"context"
	"errors"
	"fmt"

	buildreleasev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/buildrelease/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"github.com/quarkloop/services/build-release/pkg/buildrelease"
)

type Server struct {
	runner *buildrelease.Runner
}

func New(runner *buildrelease.Runner) (*Server, error) {
	if runner == nil {
		return nil, fmt.Errorf("build-release runner is required")
	}
	return &Server{runner: runner}, nil
}

func (s *Server) Release(ctx context.Context, req *buildreleasev1.ReleaseRequest) (*buildreleasev1.ReleaseResponse, error) {
	if err := validateWorkingDir(req.GetWorkingDir()); err != nil {
		return nil, grpcError(err)
	}
	result, err := s.runner.Release(ctx, releaseRequestFromProto(req))
	if err != nil {
		return nil, grpcError(err)
	}
	return releaseResponseToProto(result), nil
}

func (s *Server) DryRun(ctx context.Context, req *buildreleasev1.DryRunRequest) (*buildreleasev1.DryRunResponse, error) {
	if err := validateWorkingDir(req.GetWorkingDir()); err != nil {
		return nil, grpcError(err)
	}
	result, err := s.runner.DryRun(ctx, dryRunRequestFromProto(req))
	if err != nil {
		return nil, grpcError(err)
	}
	return dryRunResponseToProto(result), nil
}

func (s *Server) Init(ctx context.Context, req *buildreleasev1.InitRequest) (*buildreleasev1.InitResponse, error) {
	if err := validateWorkingDir(req.GetWorkingDir()); err != nil {
		return nil, grpcError(err)
	}
	result, err := s.runner.Init(ctx, initRequestFromProto(req))
	if err != nil {
		return nil, grpcError(err)
	}
	return initResponseToProto(result), nil
}

func validateWorkingDir(workingDir string) error {
	if workingDir == "" {
		return fmt.Errorf("working_dir is required")
	}
	return nil
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return serviceerrors.Canceled(err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return serviceerrors.DeadlineExceeded(err.Error())
	default:
		return serviceerrors.InvalidArgument(err.Error())
	}
}
