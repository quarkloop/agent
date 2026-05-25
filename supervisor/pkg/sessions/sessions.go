// Package sessions owns supervisor session semantics while delegating opaque
// bytes to the Space service storage boundary.
package sessions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/quarkloop/supervisor/pkg/space"
)

const recordNamespace = "sessions"

var ErrNotFound = errors.New("session not found")

type Type string

const (
	TypeMain     Type = "main"
	TypeChat     Type = "chat"
	TypeTask     Type = "task"
	TypeSubAgent Type = "subagent"
	TypeCron     Type = "cron"
)

type Session struct {
	ID        string    `json:"id"`
	Space     string    `json:"space"`
	Type      Type      `json:"type"`
	Title     string    `json:"title,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Records interface {
	PutRecord(name, namespace, key string, data []byte) error
	GetRecord(name, namespace, key string) ([]byte, error)
	ListRecords(name, namespace string) ([][]byte, error)
	DeleteRecord(name, namespace, key string) error
}

type Repository struct {
	records Records
	now     func() time.Time
	id      func() string
}

func NewRepository(records Records) (*Repository, error) {
	if records == nil {
		return nil, fmt.Errorf("session record storage is required")
	}
	return &Repository{records: records, now: time.Now, id: newID}, nil
}

func (r *Repository) Create(space string, t Type, title string) (Session, error) {
	if strings.TrimSpace(space) == "" {
		return Session{}, fmt.Errorf("space is required")
	}
	if t == "" {
		t = TypeChat
	}
	now := r.now().UTC()
	session := Session{ID: r.id(), Space: space, Type: t, Title: title, Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := r.write(session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (r *Repository) Get(space, id string) (Session, error) {
	raw, err := r.records.GetRecord(space, recordNamespace, id)
	if err != nil {
		return Session{}, mapStorageError(err)
	}
	return decode(raw)
}

func (r *Repository) List(space string) ([]Session, error) {
	rawRecords, err := r.records.ListRecords(space, recordNamespace)
	if err != nil {
		return nil, mapStorageError(err)
	}
	out := make([]Session, 0, len(rawRecords))
	for _, raw := range rawRecords {
		session, err := decode(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *Repository) Delete(space, id string) error {
	if err := r.records.DeleteRecord(space, recordNamespace, id); err != nil {
		return mapStorageError(err)
	}
	return nil
}

func (r *Repository) Touch(space, id string) error {
	session, err := r.Get(space, id)
	if err != nil {
		return err
	}
	session.UpdatedAt = r.now().UTC()
	return r.write(session)
}

func (r *Repository) write(session Session) error {
	raw, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := r.records.PutRecord(session.Space, recordNamespace, session.ID, raw); err != nil {
		return mapStorageError(err)
	}
	return nil
}

func decode(raw []byte) (Session, error) {
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, fmt.Errorf("decode stored session: %w", err)
	}
	if session.ID == "" || session.Space == "" {
		return Session{}, fmt.Errorf("stored session is missing identity")
	}
	return session, nil
}

func mapStorageError(err error) error {
	if space.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

func newID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}
