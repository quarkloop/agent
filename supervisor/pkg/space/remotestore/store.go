// Package remotestore implements supervisor space persistence through the
// canonical Space service NATS functions. It does not write space config files.
package remotestore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/natskit"
	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/kb"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/sessions"
	"github.com/quarkloop/supervisor/pkg/space"
	spacestore "github.com/quarkloop/supervisor/pkg/space/store"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const requestTimeout = 5 * time.Second

// Store delegates authoritative config operations to Space service. The
// remaining path-scoped stores are transitional control-plane consumers and
// are removed with their owning migrations (KB/Harness and supervisor state).
type Store struct {
	client *natskit.Client
}

func New(ctx context.Context, cfg natskit.Config) (*Store, error) {
	client, err := natskit.Connect(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect space persistence service: %w", err)
	}
	return &Store{client: client}, nil
}

func (s *Store) Close() {
	if s != nil && s.client != nil {
		s.client.Close()
	}
}

func (s *Store) Create(config []byte) (*space.Space, error) {
	var response spacev1.Space
	if err := s.call("", "create_space", &spacev1.CreateSpaceRequest{Config: append([]byte(nil), config...)}, &response); err != nil {
		return nil, err
	}
	return fromProto(&response), nil
}

func (s *Store) UpdateConfig(config []byte) (*space.Space, error) {
	var response spacev1.Space
	if err := s.call("", "update_config", &spacev1.UpdateConfigRequest{Config: append([]byte(nil), config...)}, &response); err != nil {
		return nil, err
	}
	return fromProto(&response), nil
}

func (s *Store) Get(name string) (*space.Space, error) {
	var response spacev1.Space
	if err := s.call(name, "get_space", &spacev1.GetSpaceRequest{Name: name}, &response); err != nil {
		return nil, err
	}
	return fromProto(&response), nil
}

func (s *Store) List() ([]*space.Space, error) {
	var response spacev1.ListSpacesResponse
	if err := s.call("control", "list_spaces", &emptypb.Empty{}, &response); err != nil {
		return nil, err
	}
	out := make([]*space.Space, 0, len(response.GetSpaces()))
	for _, item := range response.GetSpaces() {
		out = append(out, fromProto(item))
	}
	return out, nil
}

func (s *Store) Delete(name string) error {
	return s.call(name, "delete_space", &spacev1.DeleteSpaceRequest{Name: name}, &emptypb.Empty{})
}

func (s *Store) Config(name string) ([]byte, error) {
	var response spacev1.ConfigResponse
	if err := s.call(name, "get_config", &spacev1.GetConfigRequest{Name: name}, &response); err != nil {
		return nil, err
	}
	return append([]byte(nil), response.GetConfig()...), nil
}

func (s *Store) AgentEnvironment(name string) ([]string, error) {
	var response spacev1.AgentEnvironmentResponse
	if err := s.call(name, "get_agent_environment", &spacev1.GetAgentEnvironmentRequest{Name: name}, &response); err != nil {
		return nil, err
	}
	return append([]string(nil), response.GetEntries()...), nil
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

func (s *Store) ServiceStateDir(name, serviceName string) (string, error) {
	return "", fmt.Errorf("service state directories are no longer supervisor-owned: %s/%s", name, serviceName)
}

func (s *Store) Doctor(name string) (api.DoctorResponse, error) {
	var response spacev1.DoctorResponse
	if err := s.call(name, "doctor", &spacev1.DoctorRequest{Name: name}, &response); err != nil {
		return api.DoctorResponse{}, err
	}
	out := api.DoctorResponse{OK: response.GetOk(), Issues: make([]api.DoctorIssue, 0, len(response.GetIssues()))}
	for _, issue := range response.GetIssues() {
		out.Issues = append(out.Issues, api.DoctorIssue{Severity: issue.GetSeverity(), Message: issue.GetMessage()})
	}
	return out, nil
}

func (s *Store) paths(name string) (*spacev1.SpacePaths, error) {
	var response spacev1.SpacePaths
	if err := s.call(name, "get_space_paths", &spacev1.GetSpacePathsRequest{Name: name}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (s *Store) call(spaceID, function string, request proto.Message, response proto.Message) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("space persistence service client is not configured")
	}
	operation, err := natskit.ServiceOperation("space", function)
	if err != nil {
		return err
	}
	payload, err := protojson.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", function, err)
	}
	if strings.TrimSpace(spaceID) == "" {
		spaceID = "control"
	}
	envelope, err := natskit.NewRequest(natskit.NewServiceCallID(), spaceID, natskit.ActorSupervisor, json.RawMessage(payload))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	result, err := s.client.Call(ctx, operation, envelope)
	if err != nil {
		return fmt.Errorf("call space service %s: %w", function, err)
	}
	if result.Status != natskit.StatusOK {
		return mapError(result)
	}
	if err := protojson.Unmarshal(result.Payload, response); err != nil {
		return fmt.Errorf("decode %s response: %w", function, err)
	}
	return nil
}

func mapError(response natskit.ResponseEnvelope) error {
	if response.Error == nil {
		return fmt.Errorf("space service returned an unsuccessful response")
	}
	switch response.Error.Category {
	case boundary.NotFound:
		return spacestore.NewNotFoundError(response.Error.Message)
	case boundary.Conflict:
		return spacestore.ErrAlreadyExists
	default:
		return fmt.Errorf("space service: %s", response.Error.Message)
	}
}

func fromProto(item *spacev1.Space) *space.Space {
	if item == nil {
		return nil
	}
	return &space.Space{
		Name:       item.GetName(),
		Version:    item.GetVersion(),
		WorkingDir: item.GetWorkingDir(),
		CreatedAt:  item.GetCreatedAt().AsTime(),
		UpdatedAt:  item.GetUpdatedAt().AsTime(),
	}
}
