package runstatesvc

import (
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestNATSLeaseStoreUsesCASOwnershipAndExpiredTakeover(t *testing.T) {
	ns := startJetStreamServer(t)
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(nc.Close)
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("open jetstream: %v", err)
	}
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{Bucket: "runstate_leases", TTL: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create kv: %v", err)
	}
	storeValue, err := NewNATSLeaseStore(kv)
	if err != nil {
		t.Fatalf("new nats lease store: %v", err)
	}
	store := storeValue.(*natsLeaseStore)
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
