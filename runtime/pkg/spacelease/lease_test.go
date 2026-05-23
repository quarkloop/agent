package spacelease

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func TestClaimRejectsActiveLeaseFromAnotherRuntime(t *testing.T) {
	ns := startJetStreamNATS(t)
	defer ns.Shutdown()
	ctx := context.Background()

	first, err := New(ctx, Config{URL: ns.ClientURL(), RuntimeID: "runtime-1", Bucket: "leases_a", TTL: time.Minute})
	if err != nil {
		t.Fatalf("first manager: %v", err)
	}
	defer first.Close()
	if _, err := first.Claim(ctx, "space-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	second, err := New(ctx, Config{URL: ns.ClientURL(), RuntimeID: "runtime-2", Bucket: "leases_a", TTL: time.Minute})
	if err != nil {
		t.Fatalf("second manager: %v", err)
	}
	defer second.Close()
	if _, err := second.Claim(ctx, "space-1"); err == nil {
		t.Fatal("expected active lease conflict")
	}
}

func TestClaimAllowsRenewAndReleaseByOwner(t *testing.T) {
	ns := startJetStreamNATS(t)
	defer ns.Shutdown()
	ctx := context.Background()

	manager, err := New(ctx, Config{URL: ns.ClientURL(), RuntimeID: "runtime-1", Bucket: "leases_b", TTL: time.Minute})
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	defer manager.Close()
	lease, err := manager.Claim(ctx, "space-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := lease.Renew(ctx); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := manager.kv.Get("space-1"); err == nil {
		t.Fatal("expected released lease to be deleted")
	}
}

func TestExpiredLeaseCanBeReclaimed(t *testing.T) {
	ns := startJetStreamNATS(t)
	defer ns.Shutdown()
	ctx := context.Background()

	first, err := New(ctx, Config{URL: ns.ClientURL(), RuntimeID: "runtime-1", Bucket: "leases_c", TTL: 150 * time.Millisecond})
	if err != nil {
		t.Fatalf("first manager: %v", err)
	}
	defer first.Close()
	if _, err := first.Claim(ctx, "space-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	second, err := New(ctx, Config{URL: ns.ClientURL(), RuntimeID: "runtime-2", Bucket: "leases_c", TTL: time.Minute})
	if err != nil {
		t.Fatalf("second manager: %v", err)
	}
	defer second.Close()
	if _, err := second.Claim(ctx, "space-1"); err != nil {
		t.Fatalf("reclaim expired lease: %v", err)
	}
}

func startJetStreamNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatal("nats server did not become ready")
	}
	return ns
}
