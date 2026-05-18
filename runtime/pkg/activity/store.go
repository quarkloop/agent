// Package activity stores runtime-local activity events for user inspection.
package activity

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/quarkloop/pkg/boundary/redaction"
)

// Record is one runtime activity event.
type Record struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id,omitempty"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// Store keeps a bounded in-memory event log and live subscribers.
type Store struct {
	mu          sync.RWMutex
	next        uint64
	limit       int
	records     []Record
	subscribers map[chan Record]struct{}
}

// NewStore creates a bounded activity store.
func NewStore(limit int) *Store {
	if limit <= 0 {
		limit = 1000
	}
	return &Store{
		limit:       limit,
		subscribers: make(map[chan Record]struct{}),
	}
}

// Add appends a record and notifies subscribers. Data is marshaled at the
// boundary so mutable caller state cannot leak into the activity store.
func (s *Store) Add(sessionID, typ string, data any) Record {
	payload, _ := json.Marshal(data)
	payload = redaction.RedactBytes(payload)

	s.mu.Lock()
	s.next++
	record := Record{
		ID:        fmt.Sprintf("activity-%d", s.next),
		SessionID: sessionID,
		Type:      typ,
		Timestamp: time.Now().UTC(),
		Data:      append(json.RawMessage(nil), payload...),
	}
	s.records = append(s.records, record)
	if len(s.records) > s.limit {
		copy(s.records, s.records[len(s.records)-s.limit:])
		s.records = s.records[:s.limit]
	}
	subs := make([]chan Record, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- cloneRecord(record):
		default:
		}
	}
	return cloneRecord(record)
}

// List returns up to limit records in chronological order.
func (s *Store) List(limit int) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start := 0
	if limit > 0 && limit < len(s.records) {
		start = len(s.records) - limit
	}
	out := make([]Record, 0, len(s.records)-start)
	for _, record := range s.records[start:] {
		out = append(out, cloneRecord(record))
	}
	return out
}

// Subscribe returns a channel for live activity records.
func (s *Store) Subscribe() chan Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan Record, 64)
	s.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a live activity subscriber.
func (s *Store) Unsubscribe(ch chan Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[ch]; ok {
		delete(s.subscribers, ch)
		close(ch)
	}
}

func cloneRecord(record Record) Record {
	record.Data = append(json.RawMessage(nil), record.Data...)
	return record
}
