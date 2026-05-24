package runstatesvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// leaseStore owns only short-lived coordination claims. Durable run records
// remain in recordStore so KV expiration cannot erase audit history.
type LeaseStore interface {
	Acquire(key, runID, itemID, ownerID string, expiresAt time.Time, now time.Time) (leaseRecord, error)
	Renew(key, ownerID string, revision uint64, expiresAt time.Time, now time.Time) (leaseRecord, error)
	Release(key, ownerID string, revision uint64) error
}

type natsLeaseStore struct {
	kv nats.KeyValue
}

func NewNATSLeaseStore(kv nats.KeyValue) (LeaseStore, error) {
	if kv == nil {
		return nil, fmt.Errorf("%w: runstate lease bucket is required", errInvalidArgument)
	}
	return &natsLeaseStore{kv: kv}, nil
}

func (s *natsLeaseStore) Acquire(key, runID, itemID, ownerID string, expiresAt time.Time, now time.Time) (leaseRecord, error) {
	lease := newLease(key, runID, itemID, ownerID, expiresAt)
	data, err := json.Marshal(lease)
	if err != nil {
		return leaseRecord{}, err
	}
	revision, err := s.kv.Create(key, data)
	if err == nil {
		lease.Revision = revision
		return lease, nil
	}
	entry, getErr := s.kv.Get(key)
	if getErr != nil {
		return leaseRecord{}, fmt.Errorf("%w: acquire lease: %v", errConflict, err)
	}
	current, decodeErr := decodeLease(entry.Value(), entry.Revision())
	if decodeErr != nil {
		return leaseRecord{}, decodeErr
	}
	if expiry, parseErr := time.Parse(time.RFC3339Nano, current.ExpiresAt); parseErr != nil || !expiry.After(now) {
		revision, updateErr := s.kv.Update(key, data, entry.Revision())
		if updateErr == nil {
			lease.Revision = revision
			return lease, nil
		}
	}
	return leaseRecord{}, fmt.Errorf("%w: lease %q is held by %q", errConflict, key, current.OwnerID)
}

func (s *natsLeaseStore) Renew(key, ownerID string, revision uint64, expiresAt time.Time, _ time.Time) (leaseRecord, error) {
	entry, err := s.kv.Get(key)
	if err != nil {
		return leaseRecord{}, errNotFound
	}
	current, err := decodeLease(entry.Value(), entry.Revision())
	if err != nil {
		return leaseRecord{}, err
	}
	if current.OwnerID != ownerID || (revision != 0 && entry.Revision() != revision) {
		return leaseRecord{}, fmt.Errorf("%w: lease owner or revision does not match", errConflict)
	}
	current.ExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(current)
	if err != nil {
		return leaseRecord{}, err
	}
	next, err := s.kv.Update(key, data, entry.Revision())
	if err != nil {
		return leaseRecord{}, fmt.Errorf("%w: renew lease", errConflict)
	}
	current.Revision = next
	return current, nil
}

func (s *natsLeaseStore) Release(key, ownerID string, revision uint64) error {
	entry, err := s.kv.Get(key)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	current, err := decodeLease(entry.Value(), entry.Revision())
	if err != nil {
		return err
	}
	if current.OwnerID != ownerID || (revision != 0 && entry.Revision() != revision) {
		return fmt.Errorf("%w: lease owner or revision does not match", errConflict)
	}
	if err := s.kv.Delete(key, nats.LastRevision(entry.Revision())); err != nil {
		return fmt.Errorf("%w: release lease", errConflict)
	}
	return nil
}

func newLease(key, runID, itemID, ownerID string, expiresAt time.Time) leaseRecord {
	return leaseRecord{
		Key: key, RunID: runID, ItemID: itemID, OwnerID: ownerID,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339Nano),
	}
}

func decodeLease(data []byte, revision uint64) (leaseRecord, error) {
	var lease leaseRecord
	if err := json.Unmarshal(data, &lease); err != nil {
		return leaseRecord{}, fmt.Errorf("decode runstate lease: %w", err)
	}
	lease.Revision = revision
	return lease, nil
}

type memoryLeaseStore struct {
	mu     sync.Mutex
	next   uint64
	leases map[string]leaseRecord
}

func newMemoryLeaseStore() *memoryLeaseStore {
	return &memoryLeaseStore{leases: make(map[string]leaseRecord)}
}

func (s *memoryLeaseStore) Acquire(key, runID, itemID, ownerID string, expiresAt, now time.Time) (leaseRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.leases[key]; exists {
		expiry, _ := time.Parse(time.RFC3339Nano, current.ExpiresAt)
		if expiry.After(now) {
			return leaseRecord{}, fmt.Errorf("%w: lease %q is held by %q", errConflict, key, current.OwnerID)
		}
	}
	s.next++
	lease := newLease(key, runID, itemID, ownerID, expiresAt)
	lease.Revision = s.next
	s.leases[key] = lease
	return lease, nil
}

func (s *memoryLeaseStore) Renew(key, ownerID string, revision uint64, expiresAt, _ time.Time) (leaseRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[key]
	if !ok {
		return leaseRecord{}, errNotFound
	}
	if lease.OwnerID != ownerID || (revision != 0 && lease.Revision != revision) {
		return leaseRecord{}, errConflict
	}
	s.next++
	lease.Revision = s.next
	lease.ExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	s.leases[key] = lease
	return lease, nil
}

func (s *memoryLeaseStore) Release(key, ownerID string, revision uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[key]
	if !ok {
		return errNotFound
	}
	if strings.TrimSpace(ownerID) != lease.OwnerID || (revision != 0 && lease.Revision != revision) {
		return errConflict
	}
	delete(s.leases, key)
	return nil
}
