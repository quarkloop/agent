package grpcstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/kb"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/sessions"
	"github.com/quarkloop/supervisor/pkg/space"
	"github.com/quarkloop/supervisor/pkg/space/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Store struct {
	conn *grpc.ClientConn
	api  spacev1.SpaceServiceClient
}

func Dial(ctx context.Context, address string, opts ...grpc.DialOption) (*Store, error) {
	conn, err := servicekit.Dial(ctx, address, opts...)
	if err != nil {
		return nil, err
	}
	return &Store{conn: conn, api: spacev1.NewSpaceServiceClient(conn)}, nil
}

func (s *Store) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *Store) Create(name string, quarkfile []byte, workingDir string) (*space.Space, error) {
	ctx, cancel := opContext()
	defer cancel()
	resp, err := s.api.CreateSpace(ctx, &spacev1.CreateSpaceRequest{
		Name:       name,
		Quarkfile:  quarkfile,
		WorkingDir: workingDir,
	})
	if err != nil {
		return nil, mapError(name, err)
	}
	return fromProto(resp), nil
}

func (s *Store) UpdateQuarkfile(name string, quarkfile []byte) (*space.Space, error) {
	ctx, cancel := opContext()
	defer cancel()
	resp, err := s.api.UpdateQuarkfile(ctx, &spacev1.UpdateQuarkfileRequest{
		Name:      name,
		Quarkfile: quarkfile,
	})
	if err != nil {
		return nil, mapError(name, err)
	}
	return fromProto(resp), nil
}

func (s *Store) Get(name string) (*space.Space, error) {
	ctx, cancel := opContext()
	defer cancel()
	resp, err := s.api.GetSpace(ctx, &spacev1.GetSpaceRequest{Name: name})
	if err != nil {
		return nil, mapError(name, err)
	}
	return fromProto(resp), nil
}

func (s *Store) List() ([]*space.Space, error) {
	ctx, cancel := opContext()
	defer cancel()
	resp, err := s.api.ListSpaces(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, mapError("", err)
	}
	out := make([]*space.Space, 0, len(resp.GetSpaces()))
	for _, sp := range resp.GetSpaces() {
		out = append(out, fromProto(sp))
	}
	return out, nil
}

func (s *Store) Delete(name string) error {
	ctx, cancel := opContext()
	defer cancel()
	_, err := s.api.DeleteSpace(ctx, &spacev1.DeleteSpaceRequest{Name: name})
	return mapError(name, err)
}

func (s *Store) Quarkfile(name string) ([]byte, error) {
	ctx, cancel := opContext()
	defer cancel()
	resp, err := s.api.GetQuarkfile(ctx, &spacev1.GetQuarkfileRequest{Name: name})
	if err != nil {
		return nil, mapError(name, err)
	}
	return resp.GetQuarkfile(), nil
}

func (s *Store) AgentEnvironment(name string) ([]string, error) {
	ctx, cancel := opContext()
	defer cancel()
	resp, err := s.api.GetAgentEnvironment(ctx, &spacev1.GetAgentEnvironmentRequest{Name: name})
	if err != nil {
		return nil, mapError(name, err)
	}
	return append([]string(nil), resp.GetEntries()...), nil
}

func (s *Store) KB(name string) (kb.Store, error) {
	paths, err := s.paths(name)
	if err != nil {
		return nil, err
	}
	return kb.Open(paths.GetKbDir())
}

func (s *Store) Plugins(name string) (*pluginmanager.Installer, error) {
	paths, err := s.paths(name)
	if err != nil {
		return nil, err
	}
	return pluginmanager.NewInstaller(paths.GetPluginsDir()), nil
}

func (s *Store) Sessions(name string) (*sessions.Store, error) {
	paths, err := s.paths(name)
	if err != nil {
		return nil, err
	}
	return sessions.Open(paths.GetSessionsDir(), name)
}

func (s *Store) ServiceStateDir(name, service string) (string, error) {
	if err := spacemodel.ValidateName(service); err != nil {
		return "", err
	}
	paths, err := s.paths(name)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(paths.GetRootDir(), spacemodel.ServicesDir, service)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create service state dir: %w", err)
	}
	return dir, nil
}

func (s *Store) Doctor(name string) (api.DoctorResponse, error) {
	ctx, cancel := opContext()
	defer cancel()
	resp, err := s.api.Doctor(ctx, &spacev1.DoctorRequest{Name: name})
	if err != nil {
		return api.DoctorResponse{}, mapError(name, err)
	}
	out := api.DoctorResponse{OK: resp.GetOk(), Issues: make([]api.DoctorIssue, 0, len(resp.GetIssues()))}
	for _, issue := range resp.GetIssues() {
		out.Issues = append(out.Issues, api.DoctorIssue{
			Severity: issue.GetSeverity(),
			Message:  issue.GetMessage(),
		})
	}
	return out, nil
}

func (s *Store) paths(name string) (*spacev1.SpacePaths, error) {
	ctx, cancel := opContext()
	defer cancel()
	resp, err := s.api.GetSpacePaths(ctx, &spacev1.GetSpacePathsRequest{Name: name})
	if err != nil {
		return nil, mapError(name, err)
	}
	return resp, nil
}

func opContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func fromProto(in *spacev1.Space) *space.Space {
	if in == nil {
		return nil
	}
	createdAt := time.Time{}
	if in.GetCreatedAt() != nil {
		createdAt = in.GetCreatedAt().AsTime()
	}
	updatedAt := time.Time{}
	if in.GetUpdatedAt() != nil {
		updatedAt = in.GetUpdatedAt().AsTime()
	}
	return &space.Space{
		Metadata: spacemodel.Metadata{
			Name:       in.GetName(),
			WorkingDir: in.GetWorkingDir(),
			Version:    in.GetVersion(),
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		},
	}
}

func mapError(name string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	switch st.Code() {
	case codes.NotFound:
		return store.NewNotFoundError(name)
	case codes.AlreadyExists:
		return store.ErrAlreadyExists
	default:
		return fmt.Errorf("%s", st.Message())
	}
}
