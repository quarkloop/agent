package spacelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

const (
	DefaultBucket = "runtime_space_leases"
	DefaultTTL    = 45 * time.Second
)

type Config struct {
	URL           string
	Username      string
	Password      string
	RuntimeID     string
	Bucket        string
	TTL           time.Duration
	RenewInterval time.Duration
}

type Record struct {
	SpaceID   string `json:"space_id"`
	RuntimeID string `json:"runtime_id"`
	ClaimedAt string `json:"claimed_at"`
	RenewedAt string `json:"renewed_at"`
	ExpiresAt string `json:"expires_at"`
}

type Manager struct {
	cfg  Config
	conn *natsgo.Conn
	kv   natsgo.KeyValue
	mu   sync.Mutex
}

type Lease struct {
	SpaceID   string
	RuntimeID string
	revision  uint64
	manager   *Manager
}

func ConfigFromEnv() Config {
	return Config{
		URL:       os.Getenv("QUARK_NATS_URL"),
		Username:  os.Getenv("QUARK_NATS_USER"),
		Password:  os.Getenv("QUARK_NATS_PASSWORD"),
		RuntimeID: os.Getenv("QUARK_RUNTIME_ID"),
		Bucket:    os.Getenv("QUARK_RUNTIME_LEASE_BUCKET"),
	}
}

func New(_ context.Context, cfg Config) (*Manager, error) {
	cfg = normalizeConfig(cfg)
	if cfg.URL == "" {
		return nil, errors.New("nats url is required")
	}
	conn, err := natsgo.Connect(cfg.URL, natsgo.UserInfo(cfg.Username, cfg.Password), natsgo.Name("quark-runtime-space-lease"))
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open jetstream: %w", err)
	}
	kv, err := js.KeyValue(cfg.Bucket)
	if err != nil {
		if !errors.Is(err, natsgo.ErrBucketNotFound) {
			conn.Close()
			return nil, fmt.Errorf("open lease bucket: %w", err)
		}
		kv, err = js.CreateKeyValue(&natsgo.KeyValueConfig{
			Bucket:      cfg.Bucket,
			Description: "Quark runtime space assignment leases",
			TTL:         cfg.TTL,
			History:     1,
		})
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("create lease bucket: %w", err)
		}
	}
	if err := conn.FlushTimeout(5 * time.Second); err != nil {
		conn.Close()
		return nil, fmt.Errorf("flush lease connection: %w", err)
	}
	return &Manager{cfg: cfg, conn: conn, kv: kv}, nil
}

func (m *Manager) Close() {
	if m == nil || m.conn == nil {
		return
	}
	m.conn.Drain()
	m.conn.Close()
	m.conn = nil
}

func (m *Manager) Claim(_ context.Context, spaceID string) (*Lease, error) {
	spaceID = strings.TrimSpace(spaceID)
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := leaseKey(spaceID)
	record := m.newRecord(spaceID)
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	revision, err := m.kv.Create(key, data)
	if err == nil {
		return &Lease{SpaceID: spaceID, RuntimeID: m.cfg.RuntimeID, revision: revision, manager: m}, nil
	}
	if !errors.Is(err, natsgo.ErrKeyExists) {
		return nil, fmt.Errorf("create lease: %w", err)
	}
	entry, err := m.kv.Get(key)
	if err != nil {
		if errors.Is(err, natsgo.ErrKeyNotFound) {
			revision, err = m.kv.Create(key, data)
			if err != nil {
				return nil, fmt.Errorf("create lease after missing key: %w", err)
			}
			return &Lease{SpaceID: spaceID, RuntimeID: m.cfg.RuntimeID, revision: revision, manager: m}, nil
		}
		return nil, fmt.Errorf("read existing lease: %w", err)
	}
	existing, err := decodeRecord(entry.Value())
	if err != nil {
		return nil, err
	}
	if existing.RuntimeID != m.cfg.RuntimeID && !existing.Expired(time.Now().UTC()) {
		return nil, fmt.Errorf("space %q is already leased by runtime %q until %s", spaceID, existing.RuntimeID, existing.ExpiresAt)
	}
	revision, err = m.kv.Update(key, data, entry.Revision())
	if err != nil {
		return nil, fmt.Errorf("update lease: %w", err)
	}
	return &Lease{SpaceID: spaceID, RuntimeID: m.cfg.RuntimeID, revision: revision, manager: m}, nil
}

func (l *Lease) Renew(ctx context.Context) error {
	if l == nil || l.manager == nil {
		return errors.New("lease manager is required")
	}
	return l.manager.renew(ctx, l)
}

func (l *Lease) Release(ctx context.Context) error {
	if l == nil || l.manager == nil {
		return nil
	}
	return l.manager.release(ctx, l)
}

func (l *Lease) StartRenewal(ctx context.Context) {
	if l == nil || l.manager == nil {
		return
	}
	interval := l.manager.cfg.RenewInterval
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = l.Renew(ctx)
			}
		}
	}()
}

func (m *Manager) renew(_ context.Context, lease *Lease) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := leaseKey(lease.SpaceID)
	entry, err := m.kv.Get(key)
	if err != nil {
		return fmt.Errorf("read lease before renew: %w", err)
	}
	existing, err := decodeRecord(entry.Value())
	if err != nil {
		return err
	}
	if existing.RuntimeID != m.cfg.RuntimeID {
		return fmt.Errorf("space %q lease is owned by runtime %q", lease.SpaceID, existing.RuntimeID)
	}
	record := m.newRecord(lease.SpaceID)
	record.ClaimedAt = existing.ClaimedAt
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	revision, err := m.kv.Update(key, data, entry.Revision())
	if err != nil {
		return fmt.Errorf("renew lease: %w", err)
	}
	lease.revision = revision
	return nil
}

func (m *Manager) release(_ context.Context, lease *Lease) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := leaseKey(lease.SpaceID)
	entry, err := m.kv.Get(key)
	if err != nil {
		if errors.Is(err, natsgo.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("read lease before release: %w", err)
	}
	existing, err := decodeRecord(entry.Value())
	if err != nil {
		return err
	}
	if existing.RuntimeID != m.cfg.RuntimeID {
		return nil
	}
	if err := m.kv.Delete(key); err != nil {
		return fmt.Errorf("release lease: %w", err)
	}
	return nil
}

func (m *Manager) newRecord(spaceID string) Record {
	now := time.Now().UTC()
	return Record{
		SpaceID:   spaceID,
		RuntimeID: m.cfg.RuntimeID,
		ClaimedAt: now.Format(time.RFC3339Nano),
		RenewedAt: now.Format(time.RFC3339Nano),
		ExpiresAt: now.Add(m.cfg.TTL).Format(time.RFC3339Nano),
	}
}

func (r Record) Expired(now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	if err != nil {
		return true
	}
	return !now.Before(expiresAt)
}

func decodeRecord(data []byte) (Record, error) {
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode lease record: %w", err)
	}
	if record.SpaceID == "" || record.RuntimeID == "" {
		return Record{}, errors.New("lease record is missing space_id or runtime_id")
	}
	return record, nil
}

func leaseKey(spaceID string) string {
	return strings.ReplaceAll(strings.TrimSpace(spaceID), "/", "_")
}

func normalizeConfig(cfg Config) Config {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.Password = strings.TrimSpace(cfg.Password)
	cfg.RuntimeID = strings.TrimSpace(cfg.RuntimeID)
	if cfg.RuntimeID == "" {
		cfg.RuntimeID = "runtime-" + fmt.Sprint(time.Now().UTC().UnixNano())
	}
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	if cfg.Bucket == "" {
		cfg.Bucket = DefaultBucket
	}
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.RenewInterval <= 0 {
		cfg.RenewInterval = cfg.TTL / 3
	}
	if cfg.RenewInterval <= 0 {
		cfg.RenewInterval = 10 * time.Second
	}
	return cfg
}
