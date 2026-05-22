package services_test

import (
	"testing"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/runtime/pkg/services"
)

func TestExecutorRegistersIOProtobufDescriptorsInProductionPackage(t *testing.T) {
	executor := services.NewExecutor([]*servicev1.ServiceDescriptor{{
		Name:    "io",
		Version: "1.0.0",
		Address: "127.0.0.1:1",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:  "quark.io.v1.IOService",
			Method:   "Read",
			Request:  "quark.io.v1.ReadRequest",
			Response: "quark.io.v1.ReadResponse",
		}},
	}})

	schemas := executor.ToolSchemas()
	if len(schemas) != 1 {
		t.Fatalf("schemas = %+v, want one io_Read schema", schemas)
	}
	if schemas[0].Name != "io_Read" {
		t.Fatalf("schema name = %q, want io_Read", schemas[0].Name)
	}
	properties, ok := schemas[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("io_Read schema did not resolve protobuf properties: %+v", schemas[0].Parameters)
	}
	for _, field := range []string{"path", "startLine", "endLine"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("io_Read schema missing %q property: %+v", field, properties)
		}
	}
}
