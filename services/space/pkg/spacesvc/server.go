package spacesvc

import (
	"context"
	"errors"
	"fmt"

	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	spacemodel "github.com/quarkloop/pkg/space"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	store *Store
}

func NewServer(store *Store) (*Server, error) {
	if store == nil {
		return nil, fmt.Errorf("space store is required")
	}
	return &Server{store: store}, nil
}

func (s *Server) CreateSpace(ctx context.Context, req *spacev1.CreateSpaceRequest) (*spacev1.Space, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	space, err := s.store.Create(req.GetConfig())
	if err != nil {
		return nil, grpcError(err)
	}
	return spaceToProto(space), nil
}

func (s *Server) UpdateConfig(ctx context.Context, req *spacev1.UpdateConfigRequest) (*spacev1.Space, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	space, err := s.store.UpdateConfig(req.GetConfig())
	if err != nil {
		return nil, grpcError(err)
	}
	return spaceToProto(space), nil
}

func (s *Server) GetSpace(ctx context.Context, req *spacev1.GetSpaceRequest) (*spacev1.Space, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	space, err := s.store.Get(req.GetName())
	if err != nil {
		return nil, grpcError(err)
	}
	return spaceToProto(space), nil
}

func (s *Server) ListSpaces(ctx context.Context, _ *emptypb.Empty) (*spacev1.ListSpacesResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	spaces, err := s.store.List()
	if err != nil {
		return nil, grpcError(err)
	}
	out := &spacev1.ListSpacesResponse{Spaces: make([]*spacev1.Space, 0, len(spaces))}
	for _, space := range spaces {
		out.Spaces = append(out.Spaces, spaceToProto(space))
	}
	return out, nil
}

func (s *Server) DeleteSpace(ctx context.Context, req *spacev1.DeleteSpaceRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	if err := s.store.Delete(req.GetName()); err != nil {
		return nil, grpcError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) GetConfig(ctx context.Context, req *spacev1.GetConfigRequest) (*spacev1.ConfigResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	data, space, err := s.store.Config(req.GetName())
	if err != nil {
		return nil, grpcError(err)
	}
	return &spacev1.ConfigResponse{
		Name:      space.Name,
		Version:   space.Version,
		Config:    data,
		UpdatedAt: timestamppb.New(space.UpdatedAt),
	}, nil
}

func (s *Server) GetAgentEnvironment(ctx context.Context, req *spacev1.GetAgentEnvironmentRequest) (*spacev1.AgentEnvironmentResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	env, err := s.store.AgentEnvironment(req.GetName())
	if err != nil {
		return nil, grpcError(err)
	}
	return &spacev1.AgentEnvironmentResponse{Entries: env}, nil
}

func (s *Server) GetSpacePaths(ctx context.Context, req *spacev1.GetSpacePathsRequest) (*spacev1.SpacePaths, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	paths, err := s.store.Paths(req.GetName())
	if err != nil {
		return nil, grpcError(err)
	}
	return &spacev1.SpacePaths{
		RootDir:     paths.RootDir,
		ConfigPath:  paths.ConfigPath,
		PluginsDir:  paths.PluginsDir,
		SessionsDir: paths.SessionsDir,
	}, nil
}

func (s *Server) Doctor(ctx context.Context, req *spacev1.DoctorRequest) (*spacev1.DoctorResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, grpcError(err)
	}
	result, err := s.store.Doctor(req.GetName())
	if err != nil {
		return nil, grpcError(err)
	}
	out := &spacev1.DoctorResponse{Ok: result.OK, Issues: make([]*spacev1.DoctorIssue, 0, len(result.Issues))}
	for _, issue := range result.Issues {
		out.Issues = append(out.Issues, &spacev1.DoctorIssue{
			Severity: issue.Severity,
			Message:  issue.Message,
		})
	}
	return out, nil
}

func spaceToProto(space *spacemodel.Config) *spacev1.Space {
	if space == nil {
		return nil
	}
	return &spacev1.Space{
		Name:       space.Name,
		Version:    space.Version,
		WorkingDir: space.WorkingDir,
		CreatedAt:  timestamppb.New(space.CreatedAt),
		UpdatedAt:  timestamppb.New(space.UpdatedAt),
	}
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return serviceerrors.Canceled(err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return serviceerrors.DeadlineExceeded(err.Error())
	case errors.Is(err, ErrAlreadyExists):
		return serviceerrors.AlreadyExists(err.Error())
	case IsNotFound(err):
		return serviceerrors.NotFound(err.Error())
	default:
		return serviceerrors.InvalidArgument(err.Error())
	}
}
