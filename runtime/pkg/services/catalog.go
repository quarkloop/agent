package services

import (
	"fmt"
	"os"
	"strings"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

const (
	EnvRuntimeCatalog = "QUARK_RUNTIME_SERVICE_CATALOG"
)

func CatalogFromEnv() (*Catalog, error) {
	raw := strings.TrimSpace(os.Getenv(EnvRuntimeCatalog))
	if raw == "" {
		return nil, nil
	}
	descriptors, err := servicekit.UnmarshalRuntimeServiceCatalog([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", EnvRuntimeCatalog, err)
	}
	return NewCatalog(descriptors), nil
}

func PromptBlock(descriptors []*servicev1.ServiceDescriptor) string {
	if len(descriptors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Available Service Plugins\n\n")
	b.WriteString("The supervisor resolved the following gRPC service plugins. Call their generated service functions when a task needs service-backed behavior.\n")
	b.WriteString("Never claim that data was indexed, embedded, retrieved, released, or persisted unless the matching service function has returned successfully in this session.\n")
	b.WriteString("When a user gives multiple files or records to index, index every listed item and verify one successful persistence result per item before finalizing.\n")
	b.WriteString("\nService function arguments must match the protobuf JSON shape for that RPC request message.\n")
	b.WriteString("Call at most one service function per assistant turn. Each service function argument payload must be one valid JSON object with quoted keys and schema-compatible values; never emit comments, bare key/value syntax, or multiple JSON objects in one call.\n")
	for _, desc := range descriptors {
		fmt.Fprintf(&b, "\n### %s\n\n", desc.GetName())
		fmt.Fprintf(&b, "- Type: `%s`\n", desc.GetType())
		fmt.Fprintf(&b, "- Version: `%s`\n", desc.GetVersion())
		fmt.Fprintf(&b, "- Address: `%s`\n", desc.GetAddress())
		if len(desc.GetRpcs()) > 0 {
			b.WriteString("- Functions:\n")
			for _, rpc := range desc.GetRpcs() {
				fmt.Fprintf(&b, "  - `%s`: `%s` -> `%s`", ToolNameFor(desc.GetName(), rpc.GetMethod()), rpc.GetRequest(), rpc.GetResponse())
				if rpc.GetDescription() != "" {
					fmt.Fprintf(&b, " - %s", rpc.GetDescription())
				}
				b.WriteByte('\n')
			}
		}
		for _, skill := range desc.GetSkills() {
			if strings.TrimSpace(skill.GetMarkdown()) == "" {
				continue
			}
			fmt.Fprintf(&b, "\nService skill `%s`:\n\n%s\n", skill.GetName(), strings.TrimSpace(skill.GetMarkdown()))
		}
	}
	b.WriteString("\nUse service skills together with service function results. Do not invent service responses; call the service function and use its returned JSON.\n")
	return b.String()
}
