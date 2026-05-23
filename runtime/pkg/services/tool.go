package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/plugin"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

type ServiceFunctionSchema = plugin.ToolSchema
type Executor struct {
	descriptors   []*servicev1.ServiceDescriptor
	caller        serviceFunctionCaller
	mu            sync.RWMutex
	refTTL        time.Duration
	nextEmbedding int
	embeddings    map[string][]float32
	embeddingInfo map[string]map[string]any
	embeddingBorn map[string]time.Time
	nextContent   int
	contents      map[string]string
	contentInfo   map[string]map[string]any
	contentBorn   map[string]time.Time
	pending       map[string]struct{}
}

type resolvedRPC struct {
	rpc     *servicev1.RpcDescriptor
	address string
}

func NewExecutor(descriptors []*servicev1.ServiceDescriptor) *Executor {
	return NewExecutorWithCaller(descriptors, NewNATSCaller(NATSCallerConfigFromEnv()))
}

func NewExecutorWithCaller(descriptors []*servicev1.ServiceDescriptor, caller serviceFunctionCaller) *Executor {
	out := make([]*servicev1.ServiceDescriptor, 0, len(descriptors))
	for _, desc := range descriptors {
		out = append(out, servicekit.CloneDescriptor(desc))
	}
	return &Executor{
		descriptors:   out,
		caller:        caller,
		refTTL:        defaultReferenceTTL,
		embeddings:    make(map[string][]float32),
		embeddingInfo: make(map[string]map[string]any),
		embeddingBorn: make(map[string]time.Time),
		contents:      make(map[string]string),
		contentInfo:   make(map[string]map[string]any),
		contentBorn:   make(map[string]time.Time),
		pending:       make(map[string]struct{}),
	}
}

const (
	defaultReferenceTTL     = 30 * time.Minute
	largeResultReferenceMin = 2048
	documentTextPreviewMax  = 500
	documentPagePreviewMax  = 1
	documentPageTextMax     = 120
	contextTextPreviewMax   = 1600
	reasoningPreviewMax     = 9000
)

func (e *Executor) ToolSchemas() []ServiceFunctionSchema {
	if e == nil || len(e.descriptors) == 0 {
		return nil
	}
	schemas := make([]ServiceFunctionSchema, 0)
	for _, desc := range e.descriptors {
		for _, rpc := range desc.GetRpcs() {
			if rpc.GetStreaming() {
				continue
			}
			name := FunctionNameFor(desc.GetName(), rpc)
			description := strings.TrimSpace(rpc.GetDescription())
			if description == "" {
				description = fmt.Sprintf("Call %s/%s.", rpc.GetService(), rpc.GetMethod())
			}
			schemas = append(schemas, ServiceFunctionSchema{
				Name:        name,
				Description: description,
				Parameters:  requestParameters(rpc.GetRequest()),
			})
		}
	}
	return schemas
}

func (e *Executor) Execute(ctx context.Context, functionName, arguments string) (string, error) {
	if e == nil {
		return "", fmt.Errorf("service executor is not configured")
	}
	e.CleanupExpiredReferences(time.Now())
	resolved, err := e.resolve(functionName)
	if err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.NotFound, "resolve "+functionName, err)
	}
	rpc := resolved.rpc
	arguments, err = normalizeServiceArgumentJSON(arguments)
	if err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.InvalidArgument, "decode arguments "+functionName, err)
	}
	arguments, err = normalizeDocumentInputArguments(rpc.GetRequest(), arguments)
	if err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.InvalidArgument, "normalize document input "+functionName, err)
	}
	arguments, err = injectRuntimeContextArguments(ctx, rpc.GetRequest(), arguments)
	if err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.InvalidArgument, "inject runtime context "+functionName, err)
	}
	if err := requireRuntimeReferenceArguments(rpc.GetRequest(), arguments); err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.InvalidArgument, "validate runtime references "+functionName, err)
	}
	arguments, err = e.expandRuntimeReferences(rpc.GetRequest(), arguments)
	if err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.NotFound, "expand references "+functionName, err)
	}
	arguments, err = normalizeStringMapArguments(rpc.GetRequest(), arguments)
	if err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.InvalidArgument, "normalize arguments "+functionName, err)
	}

	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpc.GetRequest()))
	if err != nil {
		return "", fmt.Errorf("request type %s not registered: %w", rpc.GetRequest(), err)
	}
	in := dynamicpb.NewMessage(msgType.Descriptor())
	if strings.TrimSpace(arguments) != "" {
		if err := serviceRequestUnmarshalOptions().Unmarshal([]byte(arguments), in); err != nil {
			return "", boundary.Wrap(boundary.Service, boundary.InvalidArgument, "decode "+rpc.GetRequest(), err)
		}
	}

	respType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpc.GetResponse()))
	if err != nil {
		return "", fmt.Errorf("response type %s not registered: %w", rpc.GetResponse(), err)
	}

	callCtx, cancel := serviceFunctionContext(ctx, rpc)
	defer cancel()
	payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(in)
	if err != nil {
		return "", boundary.Wrap(boundary.Runtime, boundary.InvalidArgument, "encode "+rpc.GetRequest(), err)
	}
	out, err := e.invokeNATSServiceFunction(callCtx, resolved, payload, respType.Descriptor())
	if err != nil {
		return "", err
	}
	if rpc.GetResponse() == "quark.embedding.v1.EmbedResponse" {
		return e.embeddingToolResult(out)
	}
	if rpc.GetResponse() == "quark.document.v1.ExtractTextResponse" {
		return e.documentExtractTextToolResult(out, arguments)
	}
	if rpc.GetResponse() == "quark.io.v1.ReadResponse" {
		return e.ioReadToolResult(out, arguments)
	}
	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encode response: %w", err)
	}
	return e.attachResultReference(functionName, rpc.GetResponse(), data)
}

func (e *Executor) CaptureToolResult(toolName, arguments, result string) (string, error) {
	return result, nil
}

func (e *Executor) resolve(functionName string) (resolvedRPC, error) {
	for _, desc := range e.descriptors {
		for _, rpc := range desc.GetRpcs() {
			if FunctionNameFor(desc.GetName(), rpc) != functionName {
				continue
			}
			return resolvedRPC{rpc: rpc, address: desc.GetAddress()}, nil
		}
	}
	return resolvedRPC{}, fmt.Errorf("service function not found: %q", functionName)
}
