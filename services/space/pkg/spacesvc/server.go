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
		return nil, serviceError(err)
	}
	space, err := s.store.Create(req.GetConfig())
	if err != nil {
		return nil, serviceError(err)
	}
	return spaceToProto(space), nil
}

func (s *Server) UpdateConfig(ctx context.Context, req *spacev1.UpdateConfigRequest) (*spacev1.Space, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	space, err := s.store.UpdateConfig(req.GetConfig())
	if err != nil {
		return nil, serviceError(err)
	}
	return spaceToProto(space), nil
}

func (s *Server) GetSpace(ctx context.Context, req *spacev1.GetSpaceRequest) (*spacev1.Space, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	space, err := s.store.Get(req.GetName())
	if err != nil {
		return nil, serviceError(err)
	}
	return spaceToProto(space), nil
}

func (s *Server) ListSpaces(ctx context.Context, _ *emptypb.Empty) (*spacev1.ListSpacesResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	spaces, err := s.store.List()
	if err != nil {
		return nil, serviceError(err)
	}
	out := &spacev1.ListSpacesResponse{Spaces: make([]*spacev1.Space, 0, len(spaces))}
	for _, space := range spaces {
		out.Spaces = append(out.Spaces, spaceToProto(space))
	}
	return out, nil
}

func (s *Server) DeleteSpace(ctx context.Context, req *spacev1.DeleteSpaceRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	if err := s.store.Delete(req.GetName()); err != nil {
		return nil, serviceError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) GetConfig(ctx context.Context, req *spacev1.GetConfigRequest) (*spacev1.ConfigResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	data, space, err := s.store.Config(req.GetName())
	if err != nil {
		return nil, serviceError(err)
	}
	return &spacev1.ConfigResponse{
		Name:      space.Name,
		Version:   space.Version,
		Config:    data,
		UpdatedAt: timestamppb.New(space.UpdatedAt),
	}, nil
}

func (s *Server) PutRecord(ctx context.Context, req *spacev1.PutRecordRequest) (*spacev1.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	record, err := s.store.PutRecord(req.GetName(), req.GetNamespace(), req.GetKey(), req.GetData())
	if err != nil {
		return nil, serviceError(err)
	}
	return recordToProto(record), nil
}

func (s *Server) GetRecord(ctx context.Context, req *spacev1.GetRecordRequest) (*spacev1.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	record, err := s.store.GetRecord(req.GetName(), req.GetNamespace(), req.GetKey())
	if err != nil {
		return nil, serviceError(err)
	}
	return recordToProto(record), nil
}

func (s *Server) ListRecords(ctx context.Context, req *spacev1.ListRecordsRequest) (*spacev1.ListRecordsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	records, err := s.store.ListRecords(req.GetName(), req.GetNamespace())
	if err != nil {
		return nil, serviceError(err)
	}
	out := &spacev1.ListRecordsResponse{Records: make([]*spacev1.Record, 0, len(records))}
	for _, record := range records {
		out.Records = append(out.Records, recordToProto(record))
	}
	return out, nil
}

func (s *Server) DeleteRecord(ctx context.Context, req *spacev1.DeleteRecordRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	if err := s.store.DeleteRecord(req.GetName(), req.GetNamespace(), req.GetKey()); err != nil {
		return nil, serviceError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) Doctor(ctx context.Context, req *spacev1.DoctorRequest) (*spacev1.DoctorResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, serviceError(err)
	}
	result, err := s.store.Doctor(req.GetName())
	if err != nil {
		return nil, serviceError(err)
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

func recordToProto(record Record) *spacev1.Record {
	return &spacev1.Record{
		Namespace: record.Namespace,
		Key:       record.Key,
		Data:      append([]byte(nil), record.Data...),
		UpdatedAt: timestamppb.New(record.UpdatedAt),
	}
}

func serviceError(err error) error {
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
