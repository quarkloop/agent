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
	"github.com/quarkloop/supervisor/pkg/space"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const requestTimeout = 5 * time.Second

// Store delegates authoritative config and opaque record persistence to Space
// service functions. It never accesses space filesystem paths.
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

func (s *Store) PutRecord(name, namespace, key string, data []byte) error {
	return s.call(name, "put_record", &spacev1.PutRecordRequest{
		Name: name, Namespace: namespace, Key: key, Data: append([]byte(nil), data...),
	}, &spacev1.Record{})
}

func (s *Store) GetRecord(name, namespace, key string) ([]byte, error) {
	var response spacev1.Record
	if err := s.call(name, "get_record", &spacev1.GetRecordRequest{Name: name, Namespace: namespace, Key: key}, &response); err != nil {
		return nil, err
	}
	return append([]byte(nil), response.GetData()...), nil
}

func (s *Store) ListRecords(name, namespace string) ([][]byte, error) {
	var response spacev1.ListRecordsResponse
	if err := s.call(name, "list_records", &spacev1.ListRecordsRequest{Name: name, Namespace: namespace}, &response); err != nil {
		return nil, err
	}
	out := make([][]byte, 0, len(response.GetRecords()))
	for _, record := range response.GetRecords() {
		out = append(out, append([]byte(nil), record.GetData()...))
	}
	return out, nil
}

func (s *Store) DeleteRecord(name, namespace, key string) error {
	return s.call(name, "delete_record", &spacev1.DeleteRecordRequest{Name: name, Namespace: namespace, Key: key}, &emptypb.Empty{})
}

func (s *Store) Doctor(name string) (space.DoctorResult, error) {
	var response spacev1.DoctorResponse
	if err := s.call(name, "doctor", &spacev1.DoctorRequest{Name: name}, &response); err != nil {
		return space.DoctorResult{}, err
	}
	out := space.DoctorResult{OK: response.GetOk(), Issues: make([]space.DoctorIssue, 0, len(response.GetIssues()))}
	for _, issue := range response.GetIssues() {
		out.Issues = append(out.Issues, space.DoctorIssue{Severity: issue.GetSeverity(), Message: issue.GetMessage()})
	}
	return out, nil
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
		return space.NewNotFoundError(response.Error.Message)
	case boundary.Conflict:
		return space.ErrAlreadyExists
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
