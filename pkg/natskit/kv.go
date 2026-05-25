package natskit

import (
	"errors"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

var (
	ErrKeyExists   = errors.New("nats key already exists")
	ErrKeyNotFound = errors.New("nats key not found")
)

type KeyValueConfig struct {
	Bucket      string
	Description string
	TTL         time.Duration
	History     uint8
}

type KeyValueEntry struct {
	value    []byte
	revision uint64
}

func (e KeyValueEntry) Value() []byte    { return append([]byte(nil), e.value...) }
func (e KeyValueEntry) Revision() uint64 { return e.revision }

type KeyValue struct {
	store natsgo.KeyValue
}

// OpenKeyValue opens a supervisor-provisioned KV bucket for application use.
// Services and runtimes must not provision durable transport resources.
func (c *Client) OpenKeyValue(bucket string) (*KeyValue, error) {
	js, err := c.jetStream()
	if err != nil {
		return nil, err
	}
	store, err := js.KeyValue(bucket)
	if err != nil {
		return nil, fmt.Errorf("open nats key value bucket %s: %w", bucket, err)
	}
	return &KeyValue{store: store}, nil
}

// EnsureKeyValue provisions a durable KV resource. Callers must own broker
// resource lifecycle, such as supervisor setup code or isolated tests.
func (c *Client) EnsureKeyValue(cfg KeyValueConfig) (*KeyValue, error) {
	js, err := c.jetStream()
	if err != nil {
		return nil, err
	}
	store, err := js.KeyValue(cfg.Bucket)
	if err == nil {
		return &KeyValue{store: store}, nil
	}
	if !errors.Is(err, natsgo.ErrBucketNotFound) {
		return nil, fmt.Errorf("open nats key value bucket %s: %w", cfg.Bucket, err)
	}
	store, err = js.CreateKeyValue(&natsgo.KeyValueConfig{
		Bucket: cfg.Bucket, Description: cfg.Description, TTL: cfg.TTL, History: cfg.History,
	})
	if err != nil {
		return nil, fmt.Errorf("create nats key value bucket %s: %w", cfg.Bucket, err)
	}
	return &KeyValue{store: store}, nil
}

func (s *KeyValue) Create(key string, value []byte) (uint64, error) {
	revision, err := s.store.Create(key, append([]byte(nil), value...))
	return revision, translateKeyValueError(err)
}

func (s *KeyValue) Get(key string) (KeyValueEntry, error) {
	entry, err := s.store.Get(key)
	if err != nil {
		return KeyValueEntry{}, translateKeyValueError(err)
	}
	return KeyValueEntry{value: append([]byte(nil), entry.Value()...), revision: entry.Revision()}, nil
}

func (s *KeyValue) Update(key string, value []byte, revision uint64) (uint64, error) {
	next, err := s.store.Update(key, append([]byte(nil), value...), revision)
	return next, translateKeyValueError(err)
}

func (s *KeyValue) Delete(key string) error {
	return translateKeyValueError(s.store.Delete(key))
}

func (s *KeyValue) DeleteRevision(key string, revision uint64) error {
	return translateKeyValueError(s.store.Delete(key, natsgo.LastRevision(revision)))
}

func translateKeyValueError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, natsgo.ErrKeyExists):
		return ErrKeyExists
	case errors.Is(err, natsgo.ErrKeyNotFound):
		return ErrKeyNotFound
	default:
		return fmt.Errorf("nats key value operation: %w", err)
	}
}
