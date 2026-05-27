//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/quarkloop/pkg/natskit"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func requestServiceFunction(t *testing.T, ctx context.Context, conn *natskit.Client, spaceID, service, function string, req proto.Message, resp proto.Message) natskit.ResponseEnvelope {
	t.Helper()
	payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(req)
	if err != nil {
		t.Fatalf("marshal service request: %v", err)
	}
	operation, err := natskit.ServiceOperation(service, function)
	if err != nil {
		t.Fatalf("service operation: %v", err)
	}
	envelope, err := natskit.NewRequest(natskit.NewServiceCallID(), spaceID, natskit.ActorRuntime, payload)
	if err != nil {
		t.Fatalf("validate service request: %v", err)
	}
	out, err := conn.Call(ctx, operation, envelope)
	if err != nil {
		t.Fatalf("request %s: %v", operation.Subject, err)
	}
	if out.Status != natskit.StatusOK {
		t.Fatalf("service response failed: %+v", out.Error)
	}
	if out.ServiceCallID != envelope.ServiceCallID ||
		out.ReferenceID != natskit.ReferenceIDForServiceCall(envelope.ServiceCallID) ||
		out.AuditRef != natskit.AuditRefForReference(out.ReferenceID) {
		t.Fatalf("service response audit references are inconsistent: %+v", out)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(out.Payload, resp); err != nil {
		t.Fatalf("decode service payload: %v", err)
	}
	return out
}
