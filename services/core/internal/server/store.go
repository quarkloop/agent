package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const storeFileName = "core-state.json"

var errNotFound = errors.New("record not found")

type fileStore struct {
	path string

	mu   sync.Mutex
	data coreData
}

type coreData struct {
	AuditEvents    []json.RawMessage            `json:"audit_events"`
	Artifacts      map[string]json.RawMessage   `json:"artifacts"`
	Approvals      map[string]json.RawMessage   `json:"approvals"`
	Configs        map[string]json.RawMessage   `json:"configs"`
	Events         map[string][]json.RawMessage `json:"events"`
	EventSequences map[string]uint64            `json:"event_sequences"`
	Plans          map[string]json.RawMessage   `json:"plans"`
	Schedules      map[string]json.RawMessage   `json:"schedules"`
	Evaluations    map[string]json.RawMessage   `json:"evaluations"`
}

func newFileStore(root string) (*fileStore, error) {
	if root == "" {
		return nil, fmt.Errorf("core state root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create core state root: %w", err)
	}
	store := &fileStore{
		path: filepath.Join(root, storeFileName),
		data: emptyCoreData(),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *fileStore) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read core state: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	var state coreData
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parse core state: %w", err)
	}
	s.data = normalizeCoreData(state)
	return nil
}

func (s *fileStore) recordAudit(event *corev1.AuditEvent) (*corev1.AuditEvent, error) {
	cp := cloneAuditEvent(event)
	raw, err := marshalProto(cp)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneCoreData(s.data)
	next.AuditEvents = append(next.AuditEvents, raw)
	if err := s.save(next); err != nil {
		return nil, err
	}
	s.data = next
	return cp, nil
}

func (s *fileStore) listAudit(runID string, limit int) ([]*corev1.AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*corev1.AuditEvent, 0, len(s.data.AuditEvents))
	for _, raw := range s.data.AuditEvents {
		event := &corev1.AuditEvent{}
		if err := unmarshalProto(raw, event); err != nil {
			return nil, err
		}
		if runID != "" && event.GetRunId() != runID {
			continue
		}
		out = append(out, event)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *fileStore) putArtifact(artifact *corev1.Artifact) (*corev1.Artifact, error) {
	cp := cloneArtifact(artifact)
	if err := s.putByID("artifact", cp.GetId(), cp, func(next coreData) map[string]json.RawMessage { return next.Artifacts }); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *fileStore) getArtifact(id string) (*corev1.Artifact, error) {
	out := &corev1.Artifact{}
	if err := s.getByID(id, out, func(data coreData) map[string]json.RawMessage { return data.Artifacts }); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *fileStore) putApproval(approval *corev1.ApprovalRequest) (*corev1.ApprovalRequest, error) {
	cp := cloneApproval(approval)
	if err := s.putByID("approval", cp.GetId(), cp, func(next coreData) map[string]json.RawMessage { return next.Approvals }); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *fileStore) getApproval(id string) (*corev1.ApprovalRequest, error) {
	out := &corev1.ApprovalRequest{}
	if err := s.getByID(id, out, func(data coreData) map[string]json.RawMessage { return data.Approvals }); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *fileStore) putConfig(value *corev1.ConfigValue) (*corev1.ConfigValue, error) {
	cp := cloneConfig(value)
	key := configKey(cp.GetScope(), cp.GetKey())
	if err := s.putByID("config", key, cp, func(next coreData) map[string]json.RawMessage { return next.Configs }); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *fileStore) getConfig(scope, key string) (*corev1.ConfigValue, error) {
	out := &corev1.ConfigValue{}
	if err := s.getByID(configKey(scope, key), out, func(data coreData) map[string]json.RawMessage { return data.Configs }); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *fileStore) publishEvent(event *corev1.Event) (*corev1.Event, error) {
	cp := cloneEvent(event)
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneCoreData(s.data)
	sequence := next.EventSequences[cp.GetStream()] + 1
	cp.Sequence = sequence
	raw, err := marshalProto(cp)
	if err != nil {
		return nil, err
	}
	next.EventSequences[cp.GetStream()] = sequence
	next.Events[cp.GetStream()] = append(next.Events[cp.GetStream()], raw)
	if err := s.save(next); err != nil {
		return nil, err
	}
	s.data = next
	return cp, nil
}

func (s *fileStore) listEvents(stream string, after uint64, limit int) ([]*corev1.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raws := s.data.Events[stream]
	out := make([]*corev1.Event, 0, len(raws))
	for _, raw := range raws {
		event := &corev1.Event{}
		if err := unmarshalProto(raw, event); err != nil {
			return nil, err
		}
		if event.GetSequence() <= after {
			continue
		}
		out = append(out, event)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *fileStore) putPlan(plan *corev1.WorkspaceMutationPlan) (*corev1.WorkspaceMutationPlan, error) {
	cp := clonePlan(plan)
	if err := s.putByID("plan", cp.GetId(), cp, func(next coreData) map[string]json.RawMessage { return next.Plans }); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *fileStore) getPlan(id string) (*corev1.WorkspaceMutationPlan, error) {
	out := &corev1.WorkspaceMutationPlan{}
	if err := s.getByID(id, out, func(data coreData) map[string]json.RawMessage { return data.Plans }); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *fileStore) putSchedule(schedule *corev1.Schedule) (*corev1.Schedule, error) {
	cp := cloneSchedule(schedule)
	if err := s.putByID("schedule", cp.GetId(), cp, func(next coreData) map[string]json.RawMessage { return next.Schedules }); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *fileStore) listSchedules(scope string) ([]*corev1.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*corev1.Schedule, 0, len(s.data.Schedules))
	for _, raw := range s.data.Schedules {
		schedule := &corev1.Schedule{}
		if err := unmarshalProto(raw, schedule); err != nil {
			return nil, err
		}
		if scope != "" && schedule.GetScope() != scope {
			continue
		}
		out = append(out, schedule)
	}
	return out, nil
}

func (s *fileStore) putEvaluation(evaluation *corev1.Evaluation) (*corev1.Evaluation, error) {
	cp := cloneEvaluation(evaluation)
	if err := s.putByID("evaluation", cp.GetId(), cp, func(next coreData) map[string]json.RawMessage { return next.Evaluations }); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *fileStore) getEvaluation(id string) (*corev1.Evaluation, error) {
	out := &corev1.Evaluation{}
	if err := s.getByID(id, out, func(data coreData) map[string]json.RawMessage { return data.Evaluations }); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *fileStore) putByID(kind, id string, message proto.Message, table func(coreData) map[string]json.RawMessage) error {
	if id == "" {
		return fmt.Errorf("%s id is required", kind)
	}
	raw, err := marshalProto(message)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneCoreData(s.data)
	table(next)[id] = raw
	if err := s.save(next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *fileStore) getByID(id string, out proto.Message, table func(coreData) map[string]json.RawMessage) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := table(s.data)[id]
	if !ok {
		return errNotFound
	}
	return unmarshalProto(raw, out)
}

func (s *fileStore) save(data coreData) error {
	body, err := json.MarshalIndent(normalizeCoreData(data), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal core state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create core state root: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("write core state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace core state: %w", err)
	}
	return nil
}

func emptyCoreData() coreData {
	return coreData{
		Artifacts:      make(map[string]json.RawMessage),
		Approvals:      make(map[string]json.RawMessage),
		Configs:        make(map[string]json.RawMessage),
		Events:         make(map[string][]json.RawMessage),
		EventSequences: make(map[string]uint64),
		Plans:          make(map[string]json.RawMessage),
		Schedules:      make(map[string]json.RawMessage),
		Evaluations:    make(map[string]json.RawMessage),
	}
}

func normalizeCoreData(data coreData) coreData {
	next := cloneCoreData(data)
	if next.Artifacts == nil {
		next.Artifacts = make(map[string]json.RawMessage)
	}
	if next.Approvals == nil {
		next.Approvals = make(map[string]json.RawMessage)
	}
	if next.Configs == nil {
		next.Configs = make(map[string]json.RawMessage)
	}
	if next.Events == nil {
		next.Events = make(map[string][]json.RawMessage)
	}
	if next.EventSequences == nil {
		next.EventSequences = make(map[string]uint64)
	}
	if next.Plans == nil {
		next.Plans = make(map[string]json.RawMessage)
	}
	if next.Schedules == nil {
		next.Schedules = make(map[string]json.RawMessage)
	}
	if next.Evaluations == nil {
		next.Evaluations = make(map[string]json.RawMessage)
	}
	return next
}

func cloneCoreData(data coreData) coreData {
	return coreData{
		AuditEvents:    cloneRawSlice(data.AuditEvents),
		Artifacts:      cloneRawMap(data.Artifacts),
		Approvals:      cloneRawMap(data.Approvals),
		Configs:        cloneRawMap(data.Configs),
		Events:         cloneEventStreams(data.Events),
		EventSequences: cloneUintMap(data.EventSequences),
		Plans:          cloneRawMap(data.Plans),
		Schedules:      cloneRawMap(data.Schedules),
		Evaluations:    cloneRawMap(data.Evaluations),
	}
}

func cloneRawSlice(in []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(in))
	for _, raw := range in {
		out = append(out, append(json.RawMessage(nil), raw...))
	}
	return out
}

func cloneRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for key, raw := range in {
		out[key] = append(json.RawMessage(nil), raw...)
	}
	return out
}

func cloneEventStreams(in map[string][]json.RawMessage) map[string][]json.RawMessage {
	out := make(map[string][]json.RawMessage, len(in))
	for key, raws := range in {
		out[key] = cloneRawSlice(raws)
	}
	return out
}

func cloneUintMap(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func marshalProto(message proto.Message) (json.RawMessage, error) {
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("marshal proto record: %w", err)
	}
	return append(json.RawMessage(nil), data...), nil
}

func unmarshalProto(raw json.RawMessage, message proto.Message) error {
	opts := protojson.UnmarshalOptions{DiscardUnknown: false}
	if err := opts.Unmarshal(raw, message); err != nil {
		return fmt.Errorf("unmarshal proto record: %w", err)
	}
	return nil
}

func cloneAuditEvent(in *corev1.AuditEvent) *corev1.AuditEvent {
	if in == nil {
		return &corev1.AuditEvent{}
	}
	return proto.Clone(in).(*corev1.AuditEvent)
}

func cloneArtifact(in *corev1.Artifact) *corev1.Artifact {
	if in == nil {
		return &corev1.Artifact{}
	}
	return proto.Clone(in).(*corev1.Artifact)
}

func cloneApproval(in *corev1.ApprovalRequest) *corev1.ApprovalRequest {
	if in == nil {
		return &corev1.ApprovalRequest{}
	}
	return proto.Clone(in).(*corev1.ApprovalRequest)
}

func cloneConfig(in *corev1.ConfigValue) *corev1.ConfigValue {
	if in == nil {
		return &corev1.ConfigValue{}
	}
	return proto.Clone(in).(*corev1.ConfigValue)
}

func cloneEvent(in *corev1.Event) *corev1.Event {
	if in == nil {
		return &corev1.Event{}
	}
	return proto.Clone(in).(*corev1.Event)
}

func clonePlan(in *corev1.WorkspaceMutationPlan) *corev1.WorkspaceMutationPlan {
	if in == nil {
		return &corev1.WorkspaceMutationPlan{}
	}
	return proto.Clone(in).(*corev1.WorkspaceMutationPlan)
}

func cloneSchedule(in *corev1.Schedule) *corev1.Schedule {
	if in == nil {
		return &corev1.Schedule{}
	}
	return proto.Clone(in).(*corev1.Schedule)
}

func cloneEvaluation(in *corev1.Evaluation) *corev1.Evaluation {
	if in == nil {
		return &corev1.Evaluation{}
	}
	return proto.Clone(in).(*corev1.Evaluation)
}

func configKey(scope, key string) string {
	return scope + "\x00" + key
}
