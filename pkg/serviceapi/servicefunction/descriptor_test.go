package servicefunction

import (
	"encoding/json"
	"testing"
	"time"
)

var objectSchema = json.RawMessage(`{"type":"object"}`)

func TestDescriptorValidationAndClone(t *testing.T) {
	descriptor, err := NewDescriptor("io", "ReadFile", DescriptorOptions{
		InputSchema:  objectSchema,
		OutputSchema: objectSchema,
		Risk:         RiskRead,
		Approval: ApprovalPolicy{
			Required:     true,
			Requirements: []string{"workspace-read"},
		},
		Idempotent: true,
		Timeout:    5 * time.Second,
		Examples: []Example{{
			Name:    "read",
			Request: json.RawMessage(`{"path":"README.md"}`),
		}},
		Permissions: PermissionSet{
			PublishAllow:   []string{"svc.io.v1.read_file"},
			SubscribeAllow: []string{"_INBOX.>"},
		},
	})
	if err != nil {
		t.Fatalf("new descriptor: %v", err)
	}
	if descriptor.Subject != "svc.io.v1.read_file" {
		t.Fatalf("subject = %q", descriptor.Subject)
	}
	if descriptor.Timeout(30*time.Second) != 5*time.Second {
		t.Fatalf("timeout = %s", descriptor.Timeout(30*time.Second))
	}

	clone := descriptor.Clone()
	clone.InputSchema[0] = '['
	clone.Approval.Requirements[0] = "mutated"
	clone.Permissions.PublishAllow[0] = "mutated"
	if string(descriptor.InputSchema) != string(objectSchema) {
		t.Fatalf("descriptor input schema reused backing array: %s", descriptor.InputSchema)
	}
	if descriptor.Approval.Requirements[0] != "workspace-read" {
		t.Fatalf("descriptor approval requirements mutated: %+v", descriptor.Approval)
	}
	if descriptor.Permissions.PublishAllow[0] != "svc.io.v1.read_file" {
		t.Fatalf("descriptor permissions mutated: %+v", descriptor.Permissions)
	}
}

func TestDescriptorRejectsMalformedSchemaAndTimeout(t *testing.T) {
	descriptor := Descriptor{
		Version:       DescriptorVersion,
		Service:       "io",
		Function:      "read_file",
		Subject:       "svc.io.v1.read_file",
		InputSchema:   json.RawMessage(`{`),
		OutputSchema:  objectSchema,
		Risk:          RiskRead,
		TimeoutMillis: 1000,
	}
	if err := descriptor.Validate(); err == nil {
		t.Fatal("expected malformed schema error")
	}

	descriptor.InputSchema = objectSchema
	descriptor.TimeoutMillis = 0
	if err := descriptor.Validate(); err == nil {
		t.Fatal("expected timeout error")
	}
}
