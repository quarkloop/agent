package natshub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/natskit"
)

var ErrAuditRecordNotFound = errors.New("audit record not found")

type AuditFilter struct {
	SpaceID   string
	SessionID string
	RunID     string
	Service   string
	Function  string
	Limit     int
	Cursor    uint64
}

type StoredAuditRecord struct {
	Sequence           uint64
	ServiceCallID      string
	ReferenceID        string
	AuditRef           string
	SpaceID            string
	SessionID          string
	RunID              string
	WorkflowID         string
	AgentID            string
	Service            string
	Function           string
	Subject            string
	Status             string
	ErrorCategory      string
	DurationMillis     int64
	TraceID            string
	RequestSnapshot    json.RawMessage
	ResponseSnapshot   json.RawMessage
	RetentionExpiresAt string
	RecordedAt         string
}

type AuditPage struct {
	Records    []StoredAuditRecord
	NextCursor uint64
}

type AuditRetention struct {
	MaxAge      time.Duration
	MaxMessages int64
}

const (
	defaultAuditListLimit = 50
	maxAuditListLimit     = 500
)

func (h *Hub) GetAuditRecord(ctx context.Context, spaceID, referenceID string) (StoredAuditRecord, error) {
	if strings.TrimSpace(spaceID) == "" || strings.TrimSpace(referenceID) == "" {
		return StoredAuditRecord{}, fmt.Errorf("space_id and reference_id are required")
	}
	js, closeConn, err := h.openAuditJetStream(ctx)
	if err != nil {
		return StoredAuditRecord{}, err
	}
	defer closeConn()
	subject := natskit.ServiceCallRecordSubject("audit", spaceID, referenceID)
	message, err := js.GetLastMsg(StreamAudit, subject, nats.Context(ctx))
	if errors.Is(err, nats.ErrMsgNotFound) {
		return StoredAuditRecord{}, ErrAuditRecordNotFound
	}
	if err != nil {
		return StoredAuditRecord{}, fmt.Errorf("read audit record: %w", err)
	}
	record, err := decodeStoredAuditRecord(message)
	if err != nil {
		return StoredAuditRecord{}, err
	}
	if record.ReferenceID != referenceID || record.SpaceID != spaceID {
		return StoredAuditRecord{}, ErrAuditRecordNotFound
	}
	return record, nil
}

func (h *Hub) ListAuditRecords(ctx context.Context, filter AuditFilter) (AuditPage, error) {
	if strings.TrimSpace(filter.SpaceID) == "" {
		return AuditPage{}, fmt.Errorf("space_id is required")
	}
	filter.Limit = normalizeAuditLimit(filter.Limit)
	js, closeConn, err := h.openAuditJetStream(ctx)
	if err != nil {
		return AuditPage{}, err
	}
	defer closeConn()
	subject := natskit.ServiceCallRecordsSubject("audit", filter.SpaceID)
	next := filter.Cursor + 1
	if filter.Cursor == 0 {
		next = 0
	}
	page := AuditPage{Records: make([]StoredAuditRecord, 0, filter.Limit)}
	for len(page.Records) < filter.Limit {
		message, err := js.GetMsg(StreamAudit, next, nats.DirectGetNext(subject), nats.Context(ctx))
		if errors.Is(err, nats.ErrMsgNotFound) {
			return page, nil
		}
		if err != nil {
			return AuditPage{}, fmt.Errorf("list audit records: %w", err)
		}
		record, err := decodeStoredAuditRecord(message)
		if err != nil {
			return AuditPage{}, err
		}
		page.NextCursor = message.Sequence
		next = message.Sequence + 1
		if matchesAuditFilter(record, filter) {
			page.Records = append(page.Records, record)
		}
	}
	return page, nil
}

func (h *Hub) AuditRetention() AuditRetention {
	h.mu.Lock()
	defer h.mu.Unlock()
	return AuditRetention{
		MaxAge:      h.cfg.JetStream.AuditRetention,
		MaxMessages: h.cfg.JetStream.AuditMaxMessages,
	}
}

func (h *Hub) openAuditJetStream(ctx context.Context) (nats.JetStreamContext, func(), error) {
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return nil, nil, fmt.Errorf("nats hub is not started")
	}
	url := h.cfg.ExternalURL
	if h.cfg.Mode != ModeExternal && h.server != nil {
		url = h.server.ClientURL()
	}
	credential, err := h.controlCredentialLocked()
	h.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	conn, err := nats.Connect(url, nats.UserInfo(credential.Username, credential.Password), nats.Name("quark-supervisor-audit-reader"), nats.Timeout(5*time.Second))
	if err != nil {
		return nil, nil, fmt.Errorf("connect audit store: %w", err)
	}
	js, err := conn.JetStream(nats.Context(ctx))
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("open audit store: %w", err)
	}
	return js, conn.Close, nil
}

func decodeStoredAuditRecord(message *nats.RawStreamMsg) (StoredAuditRecord, error) {
	var event natskit.ServiceCallEvent
	if err := json.Unmarshal(message.Data, &event); err != nil {
		return StoredAuditRecord{}, fmt.Errorf("decode audit record: %w", err)
	}
	return StoredAuditRecord{
		Sequence:           message.Sequence,
		ServiceCallID:      event.ServiceCallID,
		ReferenceID:        event.ReferenceID,
		AuditRef:           event.AuditRef,
		SpaceID:            event.SpaceID,
		SessionID:          event.SessionID,
		RunID:              event.RunID,
		WorkflowID:         event.WorkflowID,
		AgentID:            event.AgentID,
		Service:            event.Service,
		Function:           event.Function,
		Subject:            event.Subject,
		Status:             event.Status,
		ErrorCategory:      event.ErrorCategory,
		DurationMillis:     event.DurationMillis,
		TraceID:            event.TraceID,
		RequestSnapshot:    append(json.RawMessage(nil), event.RequestSnapshot...),
		ResponseSnapshot:   append(json.RawMessage(nil), event.ResponseSnapshot...),
		RetentionExpiresAt: event.RetentionExpiresAt,
		RecordedAt:         event.RecordedAt,
	}, nil
}

func matchesAuditFilter(record StoredAuditRecord, filter AuditFilter) bool {
	return (filter.SessionID == "" || record.SessionID == filter.SessionID) &&
		(filter.RunID == "" || record.RunID == filter.RunID) &&
		(filter.Service == "" || record.Service == filter.Service) &&
		(filter.Function == "" || record.Function == filter.Function)
}

func normalizeAuditLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultAuditListLimit
	case limit > maxAuditListLimit:
		return maxAuditListLimit
	default:
		return limit
	}
}
