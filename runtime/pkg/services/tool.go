package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/plugin"
	buildreleasev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/buildrelease/v1"
	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	ingestionv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/ingestion/v1"
	iov1 "github.com/quarkloop/pkg/serviceapi/gen/quark/io/v1"
	memoryv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/memory/v1"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ServiceFunctionSchema = plugin.ToolSchema

var _ = []any{
	indexerv1.File_quark_indexer_v1_indexer_proto,
	embeddingv1.File_quark_embedding_v1_embedding_proto,
	buildreleasev1.File_quark_buildrelease_v1_build_release_proto,
	devopsv1.File_quark_devops_v1_devops_proto,
	corev1.File_quark_core_v1_core_proto,
	documentv1.File_quark_document_v1_document_proto,
	iov1.File_quark_io_v1_io_proto,
	ingestionv1.File_quark_ingestion_v1_ingestion_proto,
	citationv1.File_quark_citation_v1_citation_proto,
	memoryv1.File_quark_memory_v1_memory_proto,
	modelv1.File_quark_model_v1_model_proto,
	spacev1.File_quark_space_v1_space_proto,
	systemv1.File_quark_system_v1_system_proto,
	emptypb.File_google_protobuf_empty_proto,
}

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

var serviceToolUnsafeChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func FunctionNameFor(serviceName string, rpc *servicev1.RpcDescriptor) string {
	if rpc != nil && strings.TrimSpace(rpc.GetFunctionName()) != "" {
		return strings.TrimSpace(rpc.GetFunctionName())
	}
	if rpc == nil {
		return ToolNameFor(serviceName, "")
	}
	return ToolNameFor(serviceName, rpc.GetMethod())
}

func ToolNameFor(serviceName, method string) string {
	serviceName = strings.TrimSpace(serviceName)
	method = strings.TrimSpace(method)
	if serviceName == "" && method == "" {
		return "service_call"
	}
	if serviceName == "" {
		serviceName = "service"
	}
	name := serviceToolUnsafeChars.ReplaceAllString(serviceName+"_"+method, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return "service_call"
	}
	return name
}

func serviceFunctionContext(ctx context.Context, rpc *servicev1.RpcDescriptor) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := time.Duration(0)
	if rpc != nil && rpc.GetTimeoutMillis() > 0 {
		timeout = time.Duration(rpc.GetTimeoutMillis()) * time.Millisecond
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (e *Executor) invokeNATSServiceFunction(ctx context.Context, resolved resolvedRPC, payload json.RawMessage, response protoreflect.MessageDescriptor) (*dynamicpb.Message, error) {
	if e == nil || e.caller == nil {
		return nil, boundary.New(boundary.Runtime, boundary.Unavailable, "service function", "NATS service function caller is not configured")
	}
	subject, serviceName, functionName, err := serviceFunctionSubject(resolved)
	if err != nil {
		return nil, boundary.Wrap(boundary.Service, boundary.InvalidArgument, "service function subject", err)
	}
	attempts := serviceFunctionMaxAttempts(resolved.rpc)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		envelope, err := e.caller.Call(ctx, serviceFunctionCall{
			Subject:  subject,
			Service:  serviceName,
			Function: functionName,
			Payload:  payload,
			RPC:      resolved.rpc,
		})
		if err == nil && envelope.Status == servicefunction.StatusOK {
			out := dynamicpb.NewMessage(response)
			if len(envelope.Payload) > 0 {
				if err := protojson.Unmarshal(envelope.Payload, out); err != nil {
					return nil, boundary.Wrap(boundary.Service, boundary.InvalidArgument, subject, err)
				}
			}
			return out, nil
		}
		if err == nil && envelope.Error != nil {
			err = boundary.New(envelope.Error.Boundary, envelope.Error.Category, envelope.Error.Operation, envelope.Error.Message)
		}
		if err == nil {
			err = boundary.New(boundary.Service, boundary.Unknown, subject, "service function returned non-ok response without an error payload")
		}
		lastErr = err
		if attempt == attempts || !serviceFunctionRetryable(resolved.rpc, err) {
			return nil, boundary.FromError(boundary.Service, subject, err)
		}
		if err := waitServiceFunctionRetry(ctx, resolved.rpc, attempt); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func serviceFunctionSubject(resolved resolvedRPC) (subject string, serviceName string, functionName string, err error) {
	rpc := resolved.rpc
	if rpc == nil {
		return "", "", "", fmt.Errorf("rpc descriptor is required")
	}
	serviceName = strings.TrimSpace(rpc.GetOwner())
	if serviceName == "" {
		serviceName = serviceNameFromFunctionName(rpc.GetFunctionName())
	}
	if serviceName == "" {
		serviceName = serviceNameFromProtoService(rpc.GetService())
	}
	if serviceName == "" {
		return "", "", "", fmt.Errorf("service owner is required for %s/%s", rpc.GetService(), rpc.GetMethod())
	}
	functionSource := strings.TrimSpace(rpc.GetFunctionName())
	if functionSource == "" {
		functionSource = strings.TrimSpace(rpc.GetMethod())
	}
	subject, err = servicefunction.SubjectFromOwnerAndFunctionName(serviceName, functionSource)
	if err != nil {
		return "", "", "", err
	}
	functionName, err = servicefunction.FunctionTokenFromOwnerAndFunctionName(serviceName, functionSource)
	if err != nil {
		return "", "", "", err
	}
	return subject, serviceName, functionName, nil
}

func serviceNameFromFunctionName(functionName string) string {
	owner, _, ok := strings.Cut(strings.TrimSpace(functionName), "_")
	if !ok {
		return ""
	}
	return owner
}

func serviceNameFromProtoService(protoService string) string {
	protoService = strings.TrimSpace(protoService)
	if protoService == "" {
		return ""
	}
	parts := strings.Split(protoService, ".")
	if len(parts) < 2 {
		return protoService
	}
	name := strings.TrimSuffix(parts[len(parts)-1], "Service")
	return name
}

func serviceFunctionMaxAttempts(rpc *servicev1.RpcDescriptor) int {
	if rpc == nil || rpc.GetRetryPolicy() == nil || rpc.GetRetryPolicy().GetMaxAttempts() <= 0 {
		return 1
	}
	return int(rpc.GetRetryPolicy().GetMaxAttempts())
}

func serviceFunctionRetryable(rpc *servicev1.RpcDescriptor, err error) bool {
	if rpc == nil || rpc.GetRetryPolicy() == nil {
		return false
	}
	code := normalizeRetryCode(serviceFunctionErrorCode(err))
	for _, retryable := range rpc.GetRetryPolicy().GetRetryableCodes() {
		if normalizeRetryCode(retryable) == code {
			return true
		}
	}
	return false
}

func serviceFunctionErrorCode(err error) string {
	var boundaryErr *boundary.Error
	if errors.As(err, &boundaryErr) {
		return string(boundaryErr.Category)
	}
	return string(boundary.Unknown)
}

func normalizeRetryCode(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, " ", "")
	return strings.ToLower(value)
}

func waitServiceFunctionRetry(ctx context.Context, rpc *servicev1.RpcDescriptor, attempt int) error {
	backoff := serviceFunctionBackoff(rpc, attempt)
	if backoff <= 0 {
		return nil
	}
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func serviceFunctionBackoff(rpc *servicev1.RpcDescriptor, attempt int) time.Duration {
	if rpc == nil || rpc.GetRetryPolicy() == nil {
		return 0
	}
	initial := time.Duration(rpc.GetRetryPolicy().GetInitialBackoffMillis()) * time.Millisecond
	if initial <= 0 {
		return 0
	}
	backoff := initial
	for i := 1; i < attempt; i++ {
		backoff *= 2
	}
	maxBackoff := time.Duration(rpc.GetRetryPolicy().GetMaxBackoffMillis()) * time.Millisecond
	if maxBackoff > 0 && backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}

func normalizeServiceArgumentJSON(arguments string) (string, error) {
	if strings.TrimSpace(arguments) == "" {
		return arguments, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err == nil {
		return arguments, nil
	} else {
		repaired := repairInvalidJSONEscapes(arguments)
		if repaired == arguments {
			return "", fmt.Errorf("decode service arguments: %w", err)
		}
		if retryErr := json.Unmarshal([]byte(repaired), &payload); retryErr != nil {
			return "", fmt.Errorf("decode service arguments: %w", err)
		}
		return repaired, nil
	}
}

func normalizeDocumentInputArguments(typeName, arguments string) (string, error) {
	if !isDocumentInputRequest(typeName) || strings.TrimSpace(arguments) == "" {
		return arguments, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode document arguments: %w", err)
	}

	input := map[string]json.RawMessage{}
	if raw, ok := payload["input"]; ok {
		if stringValue, ok := rawJSONString(raw); ok {
			input["sourceUri"] = mustJSONRaw(stringValue)
			input["filename"] = mustJSONRaw(filepath.Base(stringValue))
		} else if err := json.Unmarshal(raw, &input); err != nil {
			return "", fmt.Errorf("decode document input: %w", err)
		}
	}
	promoteDocumentInputString(payload, input, "sourceUri", "sourceUri", "source_uri", "uri", "url", "path", "filePath", "file_path", "source", "sourcePath", "source_path")
	promoteDocumentInputString(payload, input, "contentRef", "contentRef", "content_ref", "inputRef", "input_ref")
	promoteDocumentInputString(payload, input, "filename", "filename", "fileName", "file_name", "name")
	promoteDocumentInputString(payload, input, "mimeType", "mimeType", "mime_type", "mediaType", "media_type")
	if _, ok := input["filename"]; !ok {
		if raw, ok := input["sourceUri"]; ok {
			if sourceURI, ok := rawJSONString(raw); ok && strings.TrimSpace(sourceURI) != "" {
				input["filename"] = mustJSONRaw(filepath.Base(sourceURI))
			}
		}
	}
	if len(input) == 0 {
		return arguments, nil
	}
	payload["input"] = mustJSONRaw(input)
	out, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode document arguments: %w", err)
	}
	return string(out), nil
}

func isDocumentInputRequest(typeName string) bool {
	switch typeName {
	case "quark.document.v1.DetectTypeRequest",
		"quark.document.v1.ParseBytesRequest",
		"quark.document.v1.ExtractTextRequest",
		"quark.document.v1.ExtractLayoutRequest",
		"quark.document.v1.GetPagesRequest",
		"quark.document.v1.ExtractTablesRequest",
		"quark.document.v1.ExtractImagesRequest",
		"quark.document.v1.RunOCRRequest":
		return true
	default:
		return false
	}
}

func promoteDocumentInputString(payload, input map[string]json.RawMessage, target string, keys ...string) {
	if _, exists := input[target]; exists {
		return
	}
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		value, ok := rawJSONString(raw)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		input[target] = mustJSONRaw(value)
		delete(payload, key)
		return
	}
}

func rawJSONString(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func serviceRequestUnmarshalOptions() protojson.UnmarshalOptions {
	return protojson.UnmarshalOptions{DiscardUnknown: true}
}

func injectRuntimeContextArguments(ctx context.Context, typeName, arguments string) (string, error) {
	spaceID := strings.TrimSpace(modelservice.SpaceID(ctx))
	if spaceID == "" {
		return arguments, nil
	}
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(typeName))
	if err != nil {
		return arguments, nil
	}
	fields := msgType.Descriptor().Fields()
	spaceField := fields.ByName("space")
	if spaceField == nil || spaceField.Kind() != protoreflect.StringKind || spaceField.Cardinality() == protoreflect.Repeated {
		return arguments, nil
	}
	payload := make(map[string]json.RawMessage)
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
			return "", fmt.Errorf("decode service arguments for runtime context: %w", err)
		}
	}
	jsonName := string(spaceField.JSONName())
	if raw, ok := payload[jsonName]; ok {
		var existing string
		if err := json.Unmarshal(raw, &existing); err == nil && strings.TrimSpace(existing) != "" {
			return arguments, nil
		}
	}
	payload[jsonName] = mustJSONRaw(spaceID)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode service arguments with runtime context: %w", err)
	}
	return string(data), nil
}

func repairInvalidJSONEscapes(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	changed := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if ch != '\\' || i+1 >= len(input) {
			b.WriteByte(ch)
			continue
		}
		next := input[i+1]
		if isValidJSONEscapeByte(next) {
			b.WriteByte(ch)
			continue
		}
		changed = true
	}
	if !changed {
		return input
	}
	return b.String()
}

func isValidJSONEscapeByte(ch byte) bool {
	switch ch {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	default:
		return false
	}
}

func requestParameters(typeName string) map[string]any {
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(typeName))
	if err != nil {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          fmt.Sprintf("JSON protobuf request for %s.", typeName),
		}
	}

	schema := messageJSONSchema(msgType.Descriptor(), 0)
	schema["description"] = fmt.Sprintf("JSON protobuf request for %s. Use these exact JSON property names.", typeName)
	applyRuntimeReferenceFields(typeName, schema)
	if required := requiredJSONFields(typeName); len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func applyRuntimeReferenceFields(typeName string, schema map[string]any) {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	switch typeName {
	case "quark.embedding.v1.EmbedRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " For input, prefer inputRef or contentRef returned from io_Read or document_ExtractText results when embedding source files; otherwise provide explicit input. Provider, model, and dimensions are controlled by the resolved embedding service configuration."
		}
		delete(properties, "model")
		delete(properties, "dimensions")
		properties["inputRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by io_Read or document_ExtractText. Prefer this over copying source text into input.",
		}
		properties["contentRef"] = map[string]any{
			"type":        "string",
			"description": "Alias for inputRef. Reference returned by io_Read or document_ExtractText.",
		}
	case "quark.indexer.v1.IndexRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " Runtime tool calls must use embeddingRef returned from embedding_Embed; direct embedding vectors are not accepted. For textContent, prefer textContentRef returned from io_Read or document_ExtractText results when indexing source files; otherwise provide explicit textContent."
		}
		delete(properties, "embedding")
		properties["embeddingRef"] = map[string]any{
			"type":        "string",
			"description": "Required reference returned by embedding_Embed. Do not copy embedding vectors manually.",
		}
		properties["textContentRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by io_Read or document_ExtractText. Prefer this over copying source text into textContent.",
		}
	case "quark.indexer.v1.UpsertChunkRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " Runtime tool calls must use embeddingRef returned from embedding_Embed; direct embedding vectors are not accepted. For textContent, prefer textContentRef returned from io_Read or document_ExtractText results when indexing source files; otherwise provide explicit textContent. For document indexing, provide a complete canonical knowledge record: document, sourceMetadata, provenance, facts, entities, relations, and citations. Use an empty relations array only when no supported relation exists."
		}
		delete(properties, "embedding")
		applyCanonicalUpsertChunkPropertyDescriptions(properties)
		properties["embeddingRef"] = map[string]any{
			"type":        "string",
			"description": "Required reference returned by embedding_Embed. Do not copy embedding vectors manually.",
		}
		properties["textContentRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by io_Read or document_ExtractText. Prefer this over copying source text into textContent.",
		}
	case "quark.indexer.v1.QueryRequest":
		delete(properties, "queryVector")
		properties["queryVectorRef"] = map[string]any{
			"type":        "string",
			"description": "Required reference returned by embedding_Embed for the user's query. Do not copy query vectors manually.",
		}
	case "quark.citation.v1.ResolveSpansRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " Requires sourceUri, sourceText, and queries. Use only when the exact source text is available; do not call this from retrieved chunk IDs alone."
		}
	case "quark.citation.v1.VerifyGroundingRequest", "quark.citation.v1.ScoreCoverageRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " claims[].citations[] must be CitationSpan objects using only id, sourceUri, textSpan, startOffset, endOffset, and confidence. Do not use chunkId, filename, source, sourceText, or arbitrary metadata fields inside citation spans."
		}
	case "quark.citation.v1.RenderReferencesRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " citations[] must be CitationSpan objects using only id, sourceUri, textSpan, startOffset, endOffset, and confidence. Do not use chunkId, filename, source, sourceText, or arbitrary metadata fields inside citation spans."
		}
	}
}

func applyCanonicalUpsertChunkPropertyDescriptions(properties map[string]any) {
	describeObjectProperty(properties, "document", "Required source document identity with stable id, filename/name, type, sourceUri, and useful document metadata.")
	describeObjectProperty(properties, "sourceMetadata", "Required source metadata map with filename, documentId, documentName, documentType, sourceUri, sourceHash when known, and extraction/classification hints.", "minProperties", 1)
	describeObjectProperty(properties, "provenance", "Required provenance for the original source and producing agent/tool trace, including sourceUri, sourceHash when known, producedBy, ingestedAt or traceId when available.")
	describeArrayProperty(properties, "facts", "Required evidence-backed facts extracted by the agent from the source. Include subject, predicate, object, confidence, and citations for source-backed facts.", 1)
	describeArrayProperty(properties, "entities", "Required normalized people, organizations, documents, products, topics, dates, or other entities useful for retrieval and graph traversal.", 1)
	describeArrayProperty(properties, "relations", "Required relation array. Include supported relations between normalized entity IDs, or an empty array when no relation is supported by the source.", 0)
	describeArrayProperty(properties, "citations", "Required source evidence spans for the chunk or extracted facts, with sourceUri, textSpan, offsets when known, and confidence.", 1)
	describeObjectProperty(properties, "embeddingMetadata", "Embedding metadata returned by or derived from embedding_Embed, including provider, model, dimensions, and contentHash when known.")
}

func describeObjectProperty(properties map[string]any, name, description string, extras ...any) {
	property, ok := properties[name].(map[string]any)
	if !ok {
		return
	}
	property["description"] = description
	for i := 0; i+1 < len(extras); i += 2 {
		key, ok := extras[i].(string)
		if !ok || key == "" {
			continue
		}
		property[key] = extras[i+1]
	}
}

func describeArrayProperty(properties map[string]any, name, description string, minItems int) {
	property, ok := properties[name].(map[string]any)
	if !ok {
		return
	}
	property["description"] = description
	if minItems > 0 {
		property["minItems"] = minItems
	}
}

func messageJSONSchema(desc protoreflect.MessageDescriptor, depth int) map[string]any {
	properties := make(map[string]any)
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		properties[field.JSONName()] = fieldJSONSchema(field, depth+1)
	}

	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
}

func normalizeStringMapArguments(typeName, arguments string) (string, error) {
	if strings.TrimSpace(arguments) == "" {
		return arguments, nil
	}
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(typeName))
	if err != nil {
		return arguments, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode service arguments for normalization: %w", err)
	}
	if err := normalizeStringMapMessage(msgType.Descriptor(), payload); err != nil {
		return "", err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode normalized service arguments: %w", err)
	}
	return string(data), nil
}

func normalizeStringMapMessage(desc protoreflect.MessageDescriptor, payload map[string]json.RawMessage) error {
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		raw, ok := payload[field.JSONName()]
		if !ok {
			continue
		}
		switch {
		case field.IsMap() && field.Message().Fields().ByName("value").Kind() == protoreflect.StringKind:
			normalized, err := normalizeStringMapField(raw)
			if err != nil {
				return fmt.Errorf("normalize %s: %w", field.JSONName(), err)
			}
			payload[field.JSONName()] = normalized
		case field.Kind() == protoreflect.StringKind:
			normalized, err := normalizeStringField(raw)
			if err != nil {
				return fmt.Errorf("normalize %s: %w", field.JSONName(), err)
			}
			payload[field.JSONName()] = normalized
		case field.IsList() && field.Kind() == protoreflect.MessageKind:
			normalized, err := normalizeMessageList(field.Message(), raw)
			if err != nil {
				return fmt.Errorf("normalize %s: %w", field.JSONName(), err)
			}
			payload[field.JSONName()] = normalized
		case field.Kind() == protoreflect.MessageKind:
			normalized, err := normalizeMessageField(field.Message(), raw)
			if err != nil {
				return fmt.Errorf("normalize %s: %w", field.JSONName(), err)
			}
			payload[field.JSONName()] = normalized
		}
	}
	return nil
}

func normalizeStringField(raw json.RawMessage) (json.RawMessage, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return raw, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	s = stringifyStringFieldValue(value)
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func stringifyStringFieldValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		return fmt.Sprint(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if part := strings.TrimSpace(stringifyStringFieldValue(item)); part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, "\n\n")
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return text
		}
		if content, ok := typed["content"]; ok {
			return stringifyStringFieldValue(content)
		}
		data, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(data)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

func normalizeStringMapField(raw json.RawMessage) (json.RawMessage, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return raw, err
	}
	for key, value := range values {
		normalized, err := normalizeStringMapValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		values[key] = normalized
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeStringMapValue(raw json.RawMessage) (json.RawMessage, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return raw, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	switch typed := value.(type) {
	case nil:
		s = ""
	case json.Number:
		s = typed.String()
	case bool:
		s = fmt.Sprint(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		s = string(data)
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeMessageList(desc protoreflect.MessageDescriptor, raw json.RawMessage) (json.RawMessage, error) {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return raw, err
	}
	for _, item := range items {
		if err := normalizeStringMapMessage(desc, item); err != nil {
			return nil, err
		}
	}
	data, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeMessageField(desc protoreflect.MessageDescriptor, raw json.RawMessage) (json.RawMessage, error) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil {
		return raw, err
	}
	if err := normalizeStringMapMessage(desc, item); err != nil {
		return nil, err
	}
	data, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func fieldJSONSchema(field protoreflect.FieldDescriptor, depth int) map[string]any {
	if field.IsMap() {
		valueField := field.Message().Fields().ByName("value")
		return map[string]any{
			"type":                 "object",
			"additionalProperties": scalarJSONSchema(valueField, depth),
		}
	}
	if field.IsList() {
		return map[string]any{
			"type":  "array",
			"items": scalarJSONSchema(field, depth),
		}
	}
	return scalarJSONSchema(field, depth)
}

func scalarJSONSchema(field protoreflect.FieldDescriptor, depth int) map[string]any {
	switch field.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "boolean"}
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		names := make([]string, 0, values.Len())
		for i := 0; i < values.Len(); i++ {
			names = append(names, string(values.Get(i).Name()))
		}
		return map[string]any{"type": "string", "enum": names}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"type": "integer"}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return map[string]any{"type": "number"}
	case protoreflect.StringKind:
		return map[string]any{"type": "string"}
	case protoreflect.BytesKind:
		return map[string]any{"type": "string"}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if depth > 8 {
			return map[string]any{"type": "object", "additionalProperties": true}
		}
		return messageJSONSchema(field.Message(), depth)
	default:
		return map[string]any{}
	}
}

func requiredJSONFields(typeName string) []string {
	switch typeName {
	case "quark.embedding.v1.EmbedRequest":
		return nil
	case "quark.indexer.v1.IndexRequest":
		return []string{"chunkId", "embeddingRef"}
	case "quark.indexer.v1.UpsertChunkRequest":
		return []string{
			"chunkId",
			"embeddingRef",
			"document",
			"sourceMetadata",
			"provenance",
			"facts",
			"entities",
			"relations",
			"citations",
		}
	case "quark.indexer.v1.QueryRequest":
		return []string{"queryVectorRef"}
	case "quark.indexer.v1.DeleteDocumentRequest":
		return []string{"documentId"}
	case "quark.indexer.v1.DeleteChunkRequest":
		return []string{"chunkId"}
	default:
		return nil
	}
}

func (e *Executor) embeddingToolResult(msg protoreflect.ProtoMessage) (string, error) {
	reflected := msg.ProtoReflect()
	fields := reflected.Descriptor().Fields()
	vectorField := fields.ByName("vector")
	hashField := fields.ByName("content_hash")
	modelField := fields.ByName("model")
	dimensionsField := fields.ByName("dimensions")
	providerField := fields.ByName("provider")
	if vectorField == nil || hashField == nil {
		return "", fmt.Errorf("embedding response descriptor is missing expected fields")
	}

	list := reflected.Get(vectorField).List()
	vector := make([]float32, list.Len())
	for i := 0; i < list.Len(); i++ {
		vector[i] = float32(list.Get(i).Float())
	}
	contentHash := strings.TrimSpace(reflected.Get(hashField).String())
	if contentHash == "" {
		return "", fmt.Errorf("embedding response did not include contentHash")
	}

	e.mu.Lock()
	e.nextEmbedding++
	ref := fmt.Sprintf("emb_%d", e.nextEmbedding)
	now := time.Now()
	metadata := map[string]any{
		"contentHash": contentHash,
		"dimensions":  int(reflected.Get(dimensionsField).Int()),
		"model":       reflected.Get(modelField).String(),
		"provider":    reflected.Get(providerField).String(),
	}
	e.embeddings[ref] = cloneVector(vector)
	e.embeddings[contentHash] = cloneVector(vector)
	e.embeddingInfo[ref] = cloneMetadata(metadata)
	e.embeddingInfo[contentHash] = cloneMetadata(metadata)
	e.embeddingBorn[ref] = now
	e.embeddingBorn[contentHash] = now
	e.pending[ref] = struct{}{}
	e.mu.Unlock()

	payload := map[string]any{
		"embeddingRef": ref,
		"contentHash":  metadata["contentHash"],
		"dimensions":   metadata["dimensions"],
		"model":        metadata["model"],
		"provider":     metadata["provider"],
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode embedding result: %w", err)
	}
	return string(data), nil
}

func (e *Executor) documentExtractTextToolResult(msg protoreflect.ProtoMessage, requestArguments string) (string, error) {
	reflected := msg.ProtoReflect()
	fields := reflected.Descriptor().Fields()
	textField := fields.ByName("text")
	sourceHashField := fields.ByName("source_hash")
	if textField == nil {
		return "", fmt.Errorf("document extract text response descriptor is missing text field")
	}
	text := reflected.Get(textField).String()

	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("encode document text response: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return string(data), nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("decode document text response for content reference: %w", err)
	}
	sourceHash := ""
	if sourceHashField != nil {
		sourceHash = reflected.Get(sourceHashField).String()
	}
	sourceInfo := documentExtractionSourceInfo(requestArguments)
	sourceInfo["serviceFunction"] = "document_ExtractText"
	sourceInfo["sourceHash"] = sourceHash
	ref, info := e.registerContent(text, sourceInfo)
	payload["contentRef"] = mustJSONRaw(ref)
	payload["contentChars"] = mustJSONRaw(len([]rune(text)))
	payload["contentHash"] = mustJSONRaw(info["contentHash"])
	if sourceHash != "" {
		payload["sourceHash"] = mustJSONRaw(sourceHash)
	}
	compactDocumentExtractTextPayload(payload)
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode document text content reference: %w", err)
	}
	return string(out), nil
}

func (e *Executor) ioReadToolResult(msg protoreflect.ProtoMessage, requestArguments string) (string, error) {
	reflected := msg.ProtoReflect()
	contentField := reflected.Descriptor().Fields().ByName("content")
	if contentField == nil {
		return "", fmt.Errorf("io read response descriptor is missing content field")
	}
	content := reflected.Get(contentField).String()

	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("encode io read response: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return string(data), nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("decode io read response for content reference: %w", err)
	}
	sourceInfo := ioReadSourceInfo(requestArguments)
	sourceInfo["serviceFunction"] = "io_Read"
	ref, info := e.registerContent(content, sourceInfo)
	payload["contentRef"] = mustJSONRaw(ref)
	payload["contentChars"] = mustJSONRaw(len([]rune(content)))
	payload["contentHash"] = mustJSONRaw(info["contentHash"])
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode io read content reference: %w", err)
	}
	return string(out), nil
}

func ioReadSourceInfo(arguments string) map[string]any {
	info := make(map[string]any)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload); err != nil {
		return info
	}
	if rawPath, ok := payload["path"]; ok {
		var path string
		if err := json.Unmarshal(rawPath, &path); err == nil && strings.TrimSpace(path) != "" {
			info["path"] = strings.TrimSpace(path)
		}
	}
	return info
}

func documentExtractionSourceInfo(arguments string) map[string]any {
	info := make(map[string]any)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload); err != nil {
		return info
	}
	rawInput, ok := payload["input"]
	if !ok {
		return info
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(rawInput, &input); err != nil {
		return info
	}
	sourceURI := rawStringArgument(input, "sourceUri")
	if sourceURI == "" {
		sourceURI = rawStringArgument(input, "source_uri")
	}
	filename := rawStringArgument(input, "filename")
	mimeType := rawStringArgument(input, "mimeType")
	if mimeType == "" {
		mimeType = rawStringArgument(input, "mime_type")
	}
	if sourceURI != "" {
		info["sourceURI"] = sourceURI
	}
	if filename == "" {
		filename = filenameFromReference(sourceURI)
	}
	if filename != "" {
		info["filename"] = filename
	}
	if mimeType != "" {
		info["mimeType"] = mimeType
	}
	if rawMetadata, ok := input["metadata"]; ok {
		var metadata map[string]string
		if err := json.Unmarshal(rawMetadata, &metadata); err == nil {
			for key, value := range metadata {
				key = strings.TrimSpace(key)
				value = strings.TrimSpace(value)
				if key == "" || value == "" {
					continue
				}
				info[key] = value
			}
		}
	}
	return info
}

func (e *Executor) expandRuntimeReferences(typeName, arguments string) (string, error) {
	if strings.TrimSpace(arguments) == "" {
		return arguments, nil
	}
	switch typeName {
	case "quark.embedding.v1.EmbedRequest":
		arguments = e.promoteContentReference(arguments, "input", "contentRef")
		expanded, err := e.expandContentReference(arguments, "inputRef", "input")
		if err != nil {
			return "", err
		}
		expanded, err = e.expandContentReference(expanded, "contentRef", "input")
		if err != nil {
			return "", err
		}
		return stripEmbeddingRequestOverrides(expanded), nil
	case "quark.indexer.v1.IndexRequest":
		arguments = e.promoteContentReference(arguments, "textContent", "textContentRef")
		expanded, err := e.expandVectorReference(arguments, "embeddingRef", "embedding")
		if err != nil {
			return "", err
		}
		return e.expandContentReference(expanded, "textContentRef", "textContent")
	case "quark.indexer.v1.UpsertChunkRequest":
		arguments = e.promoteContentReference(arguments, "textContent", "textContentRef")
		expanded, err := e.expandVectorReference(arguments, "embeddingRef", "embedding")
		if err != nil {
			return "", err
		}
		return e.expandContentReference(expanded, "textContentRef", "textContent")
	case "quark.indexer.v1.QueryRequest":
		return e.expandVectorReference(arguments, "queryVectorRef", "queryVector")
	default:
		return arguments, nil
	}
}

func stripEmbeddingRequestOverrides(arguments string) string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return arguments
	}
	delete(payload, "model")
	delete(payload, "dimensions")
	data, err := json.Marshal(payload)
	if err != nil {
		return arguments
	}
	return string(data)
}

func requireRuntimeReferenceArguments(typeName, arguments string) error {
	switch typeName {
	case "quark.indexer.v1.IndexRequest", "quark.indexer.v1.UpsertChunkRequest":
		return requireReferenceField(typeName, arguments, "embeddingRef", "embedding")
	case "quark.indexer.v1.QueryRequest":
		return requireReferenceField(typeName, arguments, "queryVectorRef", "queryVector")
	default:
		return nil
	}
}

func requireReferenceField(typeName, arguments, refField, directField string) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return fmt.Errorf("decode service arguments: %w", err)
	}
	if raw, ok := payload[refField]; ok {
		ref, ok := singleStringArgument(raw)
		if ok && strings.TrimSpace(ref) != "" {
			return nil
		}
	}
	if _, ok := payload[directField]; ok {
		return fmt.Errorf("%s requires %s from embedding_Embed; direct %s values are not accepted in runtime tool calls", typeName, refField, directField)
	}
	return fmt.Errorf("%s requires %s from embedding_Embed", typeName, refField)
}

func (e *Executor) promoteContentReference(arguments, sourceField, refField string) string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return arguments
	}
	if _, exists := payload[refField]; exists {
		return arguments
	}
	raw, exists := payload[sourceField]
	if !exists {
		return arguments
	}
	ref, ok := singleStringArgument(raw)
	if !ok {
		return arguments
	}
	if _, exists := e.contentByRef(ref); !exists {
		return arguments
	}
	payload[refField] = mustJSONRaw(ref)
	delete(payload, sourceField)
	data, err := json.Marshal(payload)
	if err != nil {
		return arguments
	}
	return string(data)
}

func singleStringArgument(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		value = strings.TrimSpace(value)
		return value, value != ""
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil && len(values) == 1 {
		value = strings.TrimSpace(values[0])
		return value, value != ""
	}
	return "", false
}

func (e *Executor) expandContentReference(arguments, refField, contentField string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode service arguments: %w", err)
	}
	rawRef, ok := payload[refField]
	if !ok {
		return arguments, nil
	}
	var ref string
	if err := json.Unmarshal(rawRef, &ref); err != nil {
		return "", fmt.Errorf("%s must be a string: %w", refField, err)
	}
	content, ok := e.contentByRef(ref)
	if !ok {
		return "", fmt.Errorf("%s %q was not produced by an io_Read or document_ExtractText call in this runtime session", refField, ref)
	}
	rawContent, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", contentField, err)
	}
	payload[contentField] = rawContent
	delete(payload, refField)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode service arguments: %w", err)
	}
	return string(data), nil
}

func (e *Executor) NormalizeToolCallArguments(ctx context.Context, name, arguments string) (string, error) {
	_ = ctx
	if e == nil || strings.TrimSpace(name) != "indexer_UpsertChunk" {
		return arguments, nil
	}
	return e.normalizeUpsertChunkArguments(arguments)
}

func (e *Executor) normalizeUpsertChunkArguments(arguments string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode indexer_UpsertChunk arguments: %w", err)
	}
	payload = e.applyCanonicalUpsertChunkDefaults(payload)
	if !rawNonEmptyArrayArgument(payload, "citations") {
		if citation, ok := e.defaultSourceCitation(payload); ok {
			payload["citations"] = mustJSONRaw([]map[string]any{citation})
		}
	}
	if rawNonEmptyArrayArgument(payload, "citations") {
		payload = attachFactCitations(payload, payload["citations"])
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode normalized indexer_UpsertChunk arguments: %w", err)
	}
	return string(data), nil
}

func (e *Executor) applyCanonicalUpsertChunkDefaults(payload map[string]json.RawMessage) map[string]json.RawMessage {
	if payload == nil {
		return payload
	}
	textRef := rawStringArgument(payload, "textContentRef")
	text := rawStringArgument(payload, "textContent")
	var contentInfo map[string]any
	if textRef != "" {
		if info, ok := e.contentMetadataByRef(textRef); ok {
			contentInfo = info
		}
		if text == "" {
			if content, ok := e.contentByRef(textRef); ok {
				text = content
			}
		}
	}

	sourceURI := firstPayloadString(payload, contentInfo, []string{"sourceURI", "sourceUri", "source_uri", "path"}, "provenance", "document", "sourceMetadata")
	filename := firstPayloadString(payload, contentInfo, []string{"filename", "sourceFilename", "source_filename", "fileName", "file_name"}, "sourceMetadata", "document", "provenance")
	if filename == "" {
		filename = filenameFromReference(sourceURI)
	}
	documentName := firstPayloadString(payload, contentInfo, []string{"documentName", "document_name", "name", "filename"}, "document", "sourceMetadata")
	if documentName == "" {
		documentName = filename
	}
	documentType := firstPayloadString(payload, contentInfo, []string{"documentType", "document_type", "type", "mimeType", "mime_type"}, "document", "sourceMetadata")
	if documentType == "" {
		documentType = documentTypeFromFilename(filename)
	}
	sourceHash := firstPayloadString(payload, contentInfo, []string{"sourceHash", "source_hash", "contentHash", "content_hash"}, "provenance", "sourceMetadata")
	chunkID := rawStringArgument(payload, "chunkId")
	if chunkID == "" {
		chunkID = stableCanonicalID("chunk", firstNonEmptyString(sourceURI, filename, textRef, text))
		if chunkID != "" {
			payload["chunkId"] = mustJSONRaw(chunkID)
		}
	}
	documentID := firstPayloadString(payload, contentInfo, []string{"documentId", "document_id", "id"}, "document", "sourceMetadata")
	if documentID == "" {
		documentID = stableCanonicalID("doc", firstNonEmptyString(sourceURI, filename, documentName, chunkID))
	}

	if !rawObjectArgument(payload, "document") && firstNonEmptyString(documentID, documentName, sourceURI) != "" {
		payload["document"] = mustJSONRaw(map[string]any{
			"id":        documentID,
			"name":      documentName,
			"type":      documentType,
			"sourceUri": sourceURI,
			"metadata":  compactStringMap(map[string]string{"filename": filename, "mimeType": metadataString(contentInfo, "mimeType", "mime_type")}),
		})
	}
	if !rawObjectArgument(payload, "sourceMetadata") && firstNonEmptyString(filename, sourceURI, sourceHash, documentID) != "" {
		payload["sourceMetadata"] = mustJSONRaw(compactStringMap(map[string]string{
			"filename":     filename,
			"document_id":  documentID,
			"documentName": documentName,
			"documentType": documentType,
			"source_uri":   sourceURI,
			"source_hash":  sourceHash,
		}))
	}
	if !rawObjectArgument(payload, "provenance") && firstNonEmptyString(sourceURI, sourceHash) != "" {
		payload["provenance"] = mustJSONRaw(map[string]any{
			"sourceUri":  sourceURI,
			"sourceHash": sourceHash,
			"producedBy": "quark-knowledge-runtime",
			"ingestedAt": time.Now().UTC().Format(time.RFC3339),
		})
	}
	if embeddingRef := rawStringArgument(payload, "embeddingRef"); embeddingRef != "" && !rawObjectArgument(payload, "embeddingMetadata") {
		if metadata, ok := e.embeddingMetadataByRef(embeddingRef); ok {
			payload["embeddingMetadata"] = mustJSONRaw(metadata)
		}
	}
	if !rawArrayArgument(payload, "relations") {
		payload["relations"] = mustJSONRaw([]map[string]any{})
	}
	if !rawNonEmptyArrayArgument(payload, "entities") && firstNonEmptyString(documentID, documentName, filename) != "" {
		payload["entities"] = mustJSONRaw([]map[string]any{{
			"id":   documentID,
			"name": firstNonEmptyString(documentName, filename, sourceURI),
			"type": entityTypeFromDocumentType(documentType),
		}})
	}
	if !rawNonEmptyArrayArgument(payload, "facts") {
		if fact := fallbackSourceFact(documentID, firstNonEmptyString(documentName, filename, sourceURI), text); fact != nil {
			payload["facts"] = mustJSONRaw([]map[string]any{fact})
		}
	}
	return payload
}

func (e *Executor) defaultSourceCitation(payload map[string]json.RawMessage) (map[string]any, bool) {
	sourceURI := firstNestedStringArgument(payload, "provenance", "sourceUri", "source_uri")
	if sourceURI == "" {
		sourceURI = firstNestedStringArgument(payload, "document", "sourceUri", "source_uri")
	}
	if sourceURI == "" {
		sourceURI = firstMapStringArgument(payload, "sourceMetadata", "sourceUri", "source_uri", "filename")
	}
	sourceHash := firstNestedStringArgument(payload, "provenance", "sourceHash", "source_hash")
	if sourceHash == "" {
		sourceHash = firstMapStringArgument(payload, "sourceMetadata", "sourceHash", "source_hash")
	}
	text := rawStringArgument(payload, "textContent")
	if text == "" {
		ref := rawStringArgument(payload, "textContentRef")
		if ref != "" {
			if content, ok := e.contentByRef(ref); ok {
				text = content
			}
		}
	}
	textSpan := sourceCitationTextSpan(text)
	if textSpan == "" || sourceURI == "" {
		return nil, false
	}
	idSeed := sourceURI + "\n" + sourceHash + "\n" + textSpan
	sum := sha256.Sum256([]byte(idSeed))
	citation := map[string]any{
		"id":          "cite_" + hex.EncodeToString(sum[:])[:12],
		"sourceUri":   sourceURI,
		"textSpan":    textSpan,
		"startOffset": 0,
		"endOffset":   len([]rune(textSpan)),
		"confidence":  1.0,
	}
	return citation, true
}

func sourceCitationTextSpan(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	span := strings.Join(fields, " ")
	runes := []rune(span)
	if len(runes) > 240 {
		span = string(runes[:240])
	}
	return span
}

func fallbackSourceFact(documentID, documentName, text string) map[string]any {
	subject := firstNonEmptyString(documentName, documentID, "source document")
	object := sourceCitationTextSpan(text)
	if subject == "" || object == "" {
		return nil
	}
	return map[string]any{
		"id":         stableCanonicalID("fact", subject+"|contains|"+object),
		"subject":    subject,
		"predicate":  "contains",
		"object":     object,
		"confidence": 0.7,
	}
}

func firstPayloadString(payload map[string]json.RawMessage, metadata map[string]any, keys []string, objectKeys ...string) string {
	for _, objectKey := range objectKeys {
		raw, ok := payload[objectKey]
		if !ok {
			continue
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			continue
		}
		for _, key := range keys {
			if value := rawStringArgument(object, key); value != "" {
				return value
			}
		}
	}
	return metadataString(metadata, keys...)
}

func metadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		for metadataKey, raw := range metadata {
			if !strings.EqualFold(strings.TrimSpace(metadataKey), strings.TrimSpace(key)) {
				continue
			}
			switch value := raw.(type) {
			case string:
				if strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			case fmt.Stringer:
				if strings.TrimSpace(value.String()) != "" {
					return strings.TrimSpace(value.String())
				}
			}
		}
	}
	return ""
}

func compactStringMap(in map[string]string) map[string]string {
	out := make(map[string]string)
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func filenameFromReference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "file://") {
		value = strings.TrimPrefix(value, "file://")
	}
	name := filepath.Base(value)
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	return strings.TrimSpace(name)
}

func documentTypeFromFilename(filename string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if ext == "" {
		return "document"
	}
	return ext
}

func entityTypeFromDocumentType(documentType string) string {
	documentType = strings.TrimSpace(strings.ToUpper(documentType))
	switch documentType {
	case "", "DOCUMENT":
		return "DOCUMENT"
	case "APPLICATION/PDF", "PDF":
		return "DOCUMENT"
	default:
		documentType = strings.NewReplacer("/", "_", "-", "_", " ", "_").Replace(documentType)
		return documentType
	}
}

func stableCanonicalID(prefix, seed string) string {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(seed))
	return strings.TrimSpace(prefix) + "_" + hex.EncodeToString(sum[:])[:12]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func attachFactCitations(payload map[string]json.RawMessage, citations json.RawMessage) map[string]json.RawMessage {
	rawFacts, ok := payload["facts"]
	if !ok {
		return payload
	}
	var facts []map[string]json.RawMessage
	if err := json.Unmarshal(rawFacts, &facts); err != nil {
		return payload
	}
	changed := false
	for _, fact := range facts {
		if rawNonEmptyArrayArgument(fact, "citations") {
			continue
		}
		fact["citations"] = citations
		changed = true
	}
	if !changed {
		return payload
	}
	data, err := json.Marshal(facts)
	if err != nil {
		return payload
	}
	payload["facts"] = data
	return payload
}

func rawNonEmptyArrayArgument(payload map[string]json.RawMessage, key string) bool {
	raw, ok := payload[key]
	if !ok {
		return false
	}
	var values []json.RawMessage
	return json.Unmarshal(raw, &values) == nil && len(values) > 0
}

func rawArrayArgument(payload map[string]json.RawMessage, key string) bool {
	raw, ok := payload[key]
	if !ok {
		return false
	}
	var values []json.RawMessage
	return json.Unmarshal(raw, &values) == nil
}

func rawObjectArgument(payload map[string]json.RawMessage, key string) bool {
	raw, ok := payload[key]
	if !ok {
		return false
	}
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && len(value) > 0
}

func rawStringArgument(payload map[string]json.RawMessage, key string) string {
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func firstNestedStringArgument(payload map[string]json.RawMessage, objectKey string, keys ...string) string {
	raw, ok := payload[objectKey]
	if !ok {
		return ""
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return ""
	}
	for _, key := range keys {
		if value := rawStringArgument(object, key); value != "" {
			return value
		}
	}
	return ""
}

func firstMapStringArgument(payload map[string]json.RawMessage, objectKey string, keys ...string) string {
	return firstNestedStringArgument(payload, objectKey, keys...)
}

func (e *Executor) registerContent(content string, metadata map[string]any) (string, map[string]any) {
	sum := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(sum[:])
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextContent++
	ref := fmt.Sprintf("content_%d", e.nextContent)
	now := time.Now()
	info := cloneMetadata(metadata)
	info["contentHash"] = contentHash
	info["chars"] = len([]rune(content))
	e.contents[ref] = content
	e.contents[contentHash] = content
	e.contentInfo[ref] = cloneMetadata(info)
	e.contentInfo[contentHash] = cloneMetadata(info)
	e.contentBorn[ref] = now
	e.contentBorn[contentHash] = now
	return ref, cloneMetadata(info)
}

func (e *Executor) contentByRef(ref string) (string, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	content, ok := e.contents[ref]
	return content, ok
}

func (e *Executor) contentMetadataByRef(ref string) (map[string]any, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	metadata, ok := e.contentInfo[ref]
	return cloneMetadata(metadata), ok
}

func (e *Executor) attachResultReference(functionName, responseType string, data []byte) (string, error) {
	if len(data) < largeResultReferenceMin && !shouldReferenceResponse(responseType) {
		return string(data), nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return string(data), nil
	}
	ref, info := e.registerContent(string(data), map[string]any{
		"serviceFunction": functionName,
		"responseType":    responseType,
	})
	payload["resultRef"] = mustJSONRaw(ref)
	payload["resultChars"] = mustJSONRaw(info["chars"])
	payload["resultHash"] = mustJSONRaw(info["contentHash"])
	compactReferencedResultPayload(responseType, payload)
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode service result reference: %w", err)
	}
	return string(out), nil
}

func compactReferencedResultPayload(responseType string, payload map[string]json.RawMessage) {
	switch responseType {
	case "quark.indexer.v1.ContextResponse", "quark.indexer.v1.QueryContextResponse":
		compactContextResponsePayload(payload)
	}
}

func compactDocumentExtractTextPayload(payload map[string]json.RawMessage) {
	compactTextField(payload, "text", documentTextPreviewMax)
	compactDocumentPageTextArray(payload, "pages")
	payload["resultCompacted"] = mustJSONRaw(true)
}

func compactDocumentPageTextArray(payload map[string]json.RawMessage, key string) {
	raw, ok := payload[key]
	if !ok {
		return
	}
	var pages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &pages); err != nil {
		return
	}
	originalCount := len(pages)
	if originalCount > documentPagePreviewMax {
		pages = pages[:documentPagePreviewMax]
		payload[key+"Count"] = mustJSONRaw(originalCount)
		payload[key+"Truncated"] = mustJSONRaw(true)
	}
	for i := range pages {
		compactTextField(pages[i], "text", documentPageTextMax)
	}
	payload[key] = mustJSONRaw(pages)
}

func compactContextResponsePayload(payload map[string]json.RawMessage) {
	compactTextField(payload, "reasoningContext", reasoningPreviewMax)
	compactChunkArrayField(payload, "chunks")
	if raw, ok := payload["contextPackage"]; ok {
		var contextPackage map[string]json.RawMessage
		if err := json.Unmarshal(raw, &contextPackage); err == nil {
			compactChunkArrayField(contextPackage, "chunks")
			payload["contextPackage"] = mustJSONRaw(contextPackage)
		}
	}
	payload["resultCompacted"] = mustJSONRaw(true)
}

func compactChunkArrayField(payload map[string]json.RawMessage, key string) {
	raw, ok := payload[key]
	if !ok {
		return
	}
	var chunks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &chunks); err != nil {
		return
	}
	for i := range chunks {
		compactTextField(chunks[i], "text", contextTextPreviewMax)
	}
	payload[key] = mustJSONRaw(chunks)
}

func compactTextField(payload map[string]json.RawMessage, key string, maxRunes int) {
	if maxRunes <= 0 {
		return
	}
	raw, ok := payload[key]
	if !ok {
		return
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return
	}
	payload[key] = mustJSONRaw(string(runes[:maxRunes]) + "\n...[truncated; full content is available through the runtime reference in this tool result]")
	payload[key+"Chars"] = mustJSONRaw(len(runes))
	payload[key+"Truncated"] = mustJSONRaw(true)
}

func shouldReferenceResponse(responseType string) bool {
	switch responseType {
	case "quark.indexer.v1.ContextResponse",
		"quark.indexer.v1.QueryContextResponse",
		"quark.devops.v1.RunTestsResponse",
		"quark.devops.v1.ExplainFailureResponse",
		"quark.system.v1.ReadLogsResponse",
		"quark.system.v1.SnapshotResponse":
		return true
	default:
		return false
	}
}

func mustJSONRaw(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func (e *Executor) expandVectorReference(arguments, refField, vectorField string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode service arguments: %w", err)
	}
	rawRef, ok := payload[refField]
	if !ok {
		return arguments, nil
	}
	var ref string
	if err := json.Unmarshal(rawRef, &ref); err != nil {
		return "", fmt.Errorf("%s must be a string: %w", refField, err)
	}
	vector, ok := e.embeddingByRef(ref)
	if !ok {
		return "", fmt.Errorf("%s %q was not produced by embedding_Embed in this runtime session", refField, ref)
	}
	rawVector, err := json.Marshal(vector)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", vectorField, err)
	}
	payload[vectorField] = rawVector
	if vectorField == "embedding" {
		if _, ok := payload["embeddingMetadata"]; !ok {
			if metadata, ok := e.embeddingMetadataByRef(ref); ok {
				rawMetadata, err := json.Marshal(metadata)
				if err != nil {
					return "", fmt.Errorf("encode embedding metadata: %w", err)
				}
				payload["embeddingMetadata"] = rawMetadata
			}
		}
	}
	delete(payload, refField)
	e.markEmbeddingConsumed(ref)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode service arguments: %w", err)
	}
	return string(data), nil
}

func (e *Executor) embeddingMetadataByRef(ref string) (map[string]any, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	metadata, ok := e.embeddingInfo[ref]
	return cloneMetadata(metadata), ok
}

func (e *Executor) markEmbeddingConsumed(ref string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pending, ref)
}

func (e *Executor) PendingEmbeddingRefs() []string {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.pendingEmbeddingRefsLocked()
}

func (e *Executor) pendingEmbeddingRefsLocked() []string {
	refs := make([]string, 0, len(e.pending))
	for ref := range e.pending {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func (e *Executor) embeddingByRef(ref string) ([]float32, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	vector, ok := e.embeddings[ref]
	if !ok {
		return nil, false
	}
	return cloneVector(vector), true
}

func (e *Executor) CleanupExpiredReferences(now time.Time) {
	if e == nil || e.refTTL <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for ref, born := range e.contentBorn {
		if now.Sub(born) <= e.refTTL {
			continue
		}
		delete(e.contentBorn, ref)
		delete(e.contents, ref)
		delete(e.contentInfo, ref)
	}
	for ref, born := range e.embeddingBorn {
		if now.Sub(born) <= e.refTTL {
			continue
		}
		delete(e.embeddingBorn, ref)
		delete(e.embeddings, ref)
		delete(e.embeddingInfo, ref)
		delete(e.pending, ref)
	}
}

func (e *Executor) setReferenceTTL(ttl time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.refTTL = ttl
}

func cloneVector(in []float32) []float32 {
	if len(in) == 0 {
		return nil
	}
	out := make([]float32, len(in))
	copy(out, in)
	return out
}

func cloneMetadata(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
