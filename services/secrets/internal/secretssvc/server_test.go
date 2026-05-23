package secretssvc

import (
	"context"
	"strings"
	"testing"

	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
)

func TestResolveRefRedactsValueUnlessExplicitlyIncluded(t *testing.T) {
	server, err := NewServer(&fakeBackend{value: "sk-secret"}, nil)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	resp, err := server.ResolveRef(context.Background(), &secretsv1.ResolveRefRequest{
		SecretRef: "bao://secret/openrouter#api_key",
		ActorId:   "agent-1",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resp.GetSecret().GetValue() != "" || !resp.GetSecret().GetValueRedacted() {
		t.Fatalf("secret should be redacted by default: %#v", resp.GetSecret())
	}

	resp, err = server.ResolveRef(context.Background(), &secretsv1.ResolveRefRequest{
		SecretRef:    "bao://secret/openrouter#api_key",
		IncludeValue: true,
	})
	if err != nil {
		t.Fatalf("resolve with value: %v", err)
	}
	if resp.GetSecret().GetValue() != "sk-secret" || resp.GetSecret().GetValueRedacted() {
		t.Fatalf("secret value not returned when explicitly requested: %#v", resp.GetSecret())
	}
}

func TestAuditRedactsSecretMaterial(t *testing.T) {
	log := NewAuditLog()
	t.Setenv("OPENROUTER_API_KEY", "sk-test")
	id := log.Record(AuditEvent{
		SecretRef: "bao://secret/openrouter#api_key",
		Action:    "resolve",
		ActorID:   "Bearer sk-test",
		Metadata:  map[string]string{"authorization": "Bearer sk-test"},
	})
	if id == "" {
		t.Fatal("missing audit id")
	}
	event := log.events[0]
	if strings.Contains(event.ActorID, "sk-test") || strings.Contains(event.Metadata["authorization"], "sk-test") {
		t.Fatalf("audit leaked secret: %+v", event)
	}
}

func TestParseSecretRef(t *testing.T) {
	ref, err := ParseSecretRef("bao://secret/openrouter/api#key", "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ref.Mount != "secret" || ref.Path != "openrouter/api" || ref.Field != "key" {
		t.Fatalf("ref = %+v", ref)
	}
	if ref.String() != "bao://secret/openrouter/api#key" {
		t.Fatalf("string = %q", ref.String())
	}
}

type fakeBackend struct {
	value string
}

func (b *fakeBackend) Resolve(_ context.Context, ref SecretRef, includeValue bool) (*secretsv1.SecretMaterial, error) {
	secret := &secretsv1.SecretMaterial{Ref: ref.String(), Field: ref.Field, ValueRedacted: !includeValue}
	if includeValue {
		secret.Value = b.value
	}
	return secret, nil
}

func (b *fakeBackend) IssueToken(_ context.Context, req *secretsv1.IssueScopedSecretRequest) (*secretsv1.ScopedSecret, error) {
	return &secretsv1.ScopedSecret{Scope: req.GetScope(), Token: "token", Accessor: "accessor", Policies: req.GetPolicies()}, nil
}

func (b *fakeBackend) RenewLease(context.Context, string, int64) (*secretsv1.Lease, error) {
	return &secretsv1.Lease{LeaseId: "lease-1", DurationSeconds: 30, Renewable: true}, nil
}

func (b *fakeBackend) RevokeLease(context.Context, string, bool) error {
	return nil
}

func (b *fakeBackend) Rotate(context.Context, SecretRef, string, int64) (int64, error) {
	return 2, nil
}
