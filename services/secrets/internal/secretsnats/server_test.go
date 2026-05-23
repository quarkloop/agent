package secretsnats

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/services/secrets/internal/secretssvc"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestResolveRefEndpointRoundTrip(t *testing.T) {
	ns := startTestNATS(t)
	defer ns.Shutdown()

	secrets, err := secretssvc.NewServer(&fakeBackend{}, nil)
	if err != nil {
		t.Fatalf("secrets server: %v", err)
	}
	endpoints := New(Config{URL: ns.ClientURL(), Timeout: 2 * time.Second}, secrets)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := endpoints.Start(ctx); err != nil {
		t.Fatalf("start endpoints: %v", err)
	}
	defer endpoints.Close()

	nc, err := natsgo.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	payload, err := protojson.Marshal(&secretsv1.ResolveRefRequest{SecretRef: "bao://secret/openrouter#api_key", IncludeValue: true})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	subject, err := servicefunction.Subject("secrets", "v1", "resolve_ref")
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	req, err := servicefunction.NewRequest("call-1", "space-1", servicefunction.ActorRuntime, servicefunction.Descriptor{
		Service:  "secrets",
		Function: "resolve_ref",
		Subject:  subject,
	}, payload)
	if err != nil {
		t.Fatalf("request envelope: %v", err)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	msg, err := nc.Request(subject, data, 2*time.Second)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var resp servicefunction.ResponseEnvelope
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != servicefunction.StatusOK {
		t.Fatalf("response = %+v", resp)
	}
	redacted := resp.RedactedClone()
	if string(redacted.Payload) == string(resp.Payload) {
		t.Fatalf("expected redacted payload to differ")
	}
}

func startTestNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
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

type fakeBackend struct{}

func (b *fakeBackend) Resolve(_ context.Context, ref secretssvc.SecretRef, includeValue bool) (*secretsv1.SecretMaterial, error) {
	secret := &secretsv1.SecretMaterial{Ref: ref.String(), Field: ref.Field, ValueRedacted: !includeValue}
	if includeValue {
		secret.Value = "sk-test"
	}
	return secret, nil
}

func (b *fakeBackend) IssueToken(_ context.Context, req *secretsv1.IssueScopedSecretRequest) (*secretsv1.ScopedSecret, error) {
	return &secretsv1.ScopedSecret{Scope: req.GetScope(), Token: "token"}, nil
}

func (b *fakeBackend) RenewLease(context.Context, string, int64) (*secretsv1.Lease, error) {
	return &secretsv1.Lease{LeaseId: "lease-1"}, nil
}

func (b *fakeBackend) RevokeLease(context.Context, string, bool) error {
	return nil
}

func (b *fakeBackend) Rotate(context.Context, secretssvc.SecretRef, string, int64) (int64, error) {
	return 1, nil
}
