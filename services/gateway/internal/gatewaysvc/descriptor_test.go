package gatewaysvc

import (
	"strings"
	"testing"
)

func TestDescriptorMarksStreamGenerateAsStreaming(t *testing.T) {
	desc := Descriptor("127.0.0.1:0", nil)
	for _, rpc := range desc.GetRpcs() {
		if rpc.GetMethod() == "StreamGenerate" {
			if !rpc.GetStreaming() {
				t.Fatal("StreamGenerate descriptor must be marked streaming for NATS service response routing")
			}
			return
		}
	}
	t.Fatal("StreamGenerate descriptor not found")
}

func TestDescriptorPublishesCanonicalSubjectsAndOwnership(t *testing.T) {
	desc := Descriptor("127.0.0.1:0", nil)
	if len(desc.GetRpcs()) == 0 {
		t.Fatal("Gateway descriptor has no service functions")
	}
	for _, rpc := range desc.GetRpcs() {
		if rpc.GetOwner() != "gateway" {
			t.Errorf("%s owner = %q, want gateway", rpc.GetMethod(), rpc.GetOwner())
		}
		if !strings.HasPrefix(rpc.GetFunctionName(), "gateway_") {
			t.Errorf("%s function name = %q, want gateway prefix", rpc.GetMethod(), rpc.GetFunctionName())
		}
		if !strings.HasPrefix(rpc.GetSubject(), "svc.gateway.v1.") {
			t.Errorf("%s subject = %q, want canonical Gateway subject", rpc.GetMethod(), rpc.GetSubject())
		}
	}
}
