package runstatesvc

import (
	"context"
	"errors"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/natskit"
)

func TestNATSLeaseStoreUsesCASOwnershipAndExpiredTakeover(t *testing.T) {
	ns := startJetStreamServer(t)
	client, err := natskit.Connect(context.Background(), natskit.Config{URL: ns.ClientURL(), Name: "runstate-test"})
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(client.Close)
	kv, err := client.EnsureKeyValue(natskit.KeyValueConfig{Bucket: "runstate_leases", TTL: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create kv: %v", err)
	}
	storeValue, err := NewCASLeaseStore(natsTestCASStore{kv: kv})
	if err != nil {
		t.Fatalf("new nats lease store: %v", err)
	}
	store := storeValue.(*casLeaseStore)
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	first, err := store.Acquire("run.r1", "r1", "", "runtime-1", now.Add(time.Minute), now)
	if err != nil || first.Revision == 0 {
		t.Fatalf("acquire = %#v, %v", first, err)
	}
	if _, err := store.Acquire("run.r1", "r1", "", "runtime-2", now.Add(time.Minute), now); err == nil {
		t.Fatal("second owner acquired a live lease")
	}
	taken, err := store.Acquire("run.r1", "r1", "", "runtime-2", now.Add(2*time.Minute), now.Add(61*time.Second))
	if err != nil || taken.OwnerID != "runtime-2" || taken.Revision == first.Revision {
		t.Fatalf("expired takeover = %#v, %v", taken, err)
	}
	if err := store.Release(taken.Key, taken.OwnerID, taken.Revision); err != nil {
		t.Fatalf("release: %v", err)
	}
}

type natsTestCASStore struct {
	kv *natskit.KeyValue
}

func (s natsTestCASStore) Create(key string, value []byte) (uint64, error) {
	return s.kv.Create(key, value)
}

func (s natsTestCASStore) Get(key string) (CASEntry, error) {
	entry, err := s.kv.Get(key)
	if errors.Is(err, natskit.ErrKeyNotFound) {
		return CASEntry{}, ErrCASKeyNotFound
	}
	if err != nil {
		return CASEntry{}, err
	}
	return CASEntry{Value: entry.Value(), Revision: entry.Revision()}, nil
}

func (s natsTestCASStore) Update(key string, value []byte, revision uint64) (uint64, error) {
	return s.kv.Update(key, value, revision)
}

func (s natsTestCASStore) DeleteRevision(key string, revision uint64) error {
	return s.kv.DeleteRevision(key, revision)
}

func startJetStreamServer(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true,
		JetStream: true, StoreDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatal("nats server did not become ready")
	}
	t.Cleanup(func() { ns.Shutdown() })
	return ns
}
