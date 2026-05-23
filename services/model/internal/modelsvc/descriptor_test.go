package modelsvc

import "testing"

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
