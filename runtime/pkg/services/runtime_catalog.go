package services

import (
	"context"
	"fmt"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

// Catalog is the runtime view of supervisor-resolved service plugins.
type Catalog struct {
	descriptors []*servicev1.ServiceDescriptor
	executor    *Executor
}

func NewCatalog(descriptors []*servicev1.ServiceDescriptor) *Catalog {
	return NewCatalogWithCaller(descriptors, NewNATSCaller(NATSCallerConfigFromEnv()))
}

func NewCatalogWithCaller(descriptors []*servicev1.ServiceDescriptor, caller serviceFunctionCaller) *Catalog {
	copied := make([]*servicev1.ServiceDescriptor, 0, len(descriptors))
	for _, desc := range descriptors {
		if desc == nil {
			continue
		}
		copied = append(copied, servicekit.CloneDescriptor(desc))
	}
	return &Catalog{
		descriptors: copied,
		executor:    NewExecutorWithCaller(copied, caller),
	}
}

func (c *Catalog) Empty() bool {
	return c == nil || len(c.descriptors) == 0
}

func (c *Catalog) Descriptors() []*servicev1.ServiceDescriptor {
	if c == nil {
		return nil
	}
	out := make([]*servicev1.ServiceDescriptor, 0, len(c.descriptors))
	for _, desc := range c.descriptors {
		out = append(out, servicekit.CloneDescriptor(desc))
	}
	return out
}

func (c *Catalog) ToolSchemas() []ServiceFunctionSchema {
	if c == nil || c.executor == nil {
		return nil
	}
	return c.executor.ToolSchemas()
}

func (c *Catalog) Execute(ctx context.Context, name, arguments string) (string, error) {
	if c == nil || c.executor == nil || len(c.descriptors) == 0 {
		return "", fmt.Errorf("no service functions are available")
	}
	return c.executor.Execute(ctx, name, arguments)
}

func (c *Catalog) CaptureToolResult(toolName, arguments, result string) (string, error) {
	if c == nil || c.executor == nil {
		return result, nil
	}
	return c.executor.CaptureToolResult(toolName, arguments, result)
}

func (c *Catalog) NormalizeToolCallArguments(ctx context.Context, name, arguments string) (string, error) {
	if c == nil || c.executor == nil {
		return arguments, nil
	}
	return c.executor.NormalizeToolCallArguments(ctx, name, arguments)
}

func (c *Catalog) PendingEmbeddingRefs() []string {
	if c == nil || c.executor == nil {
		return nil
	}
	return c.executor.PendingEmbeddingRefs()
}
