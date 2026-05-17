package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	memoryv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/memory/v1"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
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
	mu            sync.RWMutex
	nextEmbedding int
	embeddings    map[string][]float32
	embeddingInfo map[string]map[string]any
	nextContent   int
	contents      map[string]string
	contentInfo   map[string]map[string]any
	pending       map[string]struct{}
}

type resolvedRPC struct {
	rpc     *servicev1.RpcDescriptor
	address string
}

func NewExecutor(descriptors []*servicev1.ServiceDescriptor) *Executor {
	out := make([]*servicev1.ServiceDescriptor, 0, len(descriptors))
	for _, desc := range descriptors {
		out = append(out, servicekit.CloneDescriptor(desc))
	}
	return &Executor{
		descriptors:   out,
		embeddings:    make(map[string][]float32),
		embeddingInfo: make(map[string]map[string]any),
		contents:      make(map[string]string),
		contentInfo:   make(map[string]map[string]any),
		pending:       make(map[string]struct{}),
	}
}

func (e *Executor) ToolSchemas() []ServiceFunctionSchema {
	if e == nil || len(e.descriptors) == 0 {
		return nil
	}
	schemas := make([]ServiceFunctionSchema, 0)
	for _, desc := range e.descriptors {
		if desc.GetAddress() == "" {
			continue
		}
		for _, rpc := range desc.GetRpcs() {
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
	resolved, err := e.resolve(functionName)
	if err != nil {
		return "", err
	}
	rpc := resolved.rpc
	arguments, err = normalizeServiceArgumentJSON(arguments)
	if err != nil {
		return "", err
	}
	arguments, err = e.expandRuntimeReferences(rpc.GetRequest(), arguments)
	if err != nil {
		return "", err
	}
	arguments, err = normalizeStringMapArguments(rpc.GetRequest(), arguments)
	if err != nil {
		return "", err
	}

	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpc.GetRequest()))
	if err != nil {
		return "", fmt.Errorf("request type %s not registered: %w", rpc.GetRequest(), err)
	}
	in := dynamicpb.NewMessage(msgType.Descriptor())
	if strings.TrimSpace(arguments) != "" {
		if err := protojson.Unmarshal([]byte(arguments), in); err != nil {
			return "", fmt.Errorf("decode %s: %w", rpc.GetRequest(), err)
		}
	}

	respType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpc.GetResponse()))
	if err != nil {
		return "", fmt.Errorf("response type %s not registered: %w", rpc.GetResponse(), err)
	}
	conn, err := servicekit.Dial(ctx, resolved.address)
	if err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.Transport, "dial "+resolved.address, err)
	}
	defer conn.Close()

	fullMethod := "/" + rpc.GetService() + "/" + rpc.GetMethod()
	callCtx, cancel := serviceFunctionContext(ctx, rpc)
	defer cancel()
	out, err := invokeServiceFunction(callCtx, conn, fullMethod, in, respType.Descriptor(), rpc)
	if err != nil {
		return "", boundary.Wrap(boundary.Service, boundary.Unavailable, "call "+fullMethod, err)
	}
	if rpc.GetResponse() == "quark.embedding.v1.EmbedResponse" {
		return e.embeddingToolResult(out)
	}
	if rpc.GetResponse() == "quark.document.v1.ExtractTextResponse" {
		return e.documentExtractTextToolResult(out)
	}
	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encode response: %w", err)
	}
	return string(data), nil
}

func (e *Executor) CaptureToolResult(toolName, arguments, result string) (string, error) {
	if e == nil || strings.TrimSpace(toolName) != "fs" {
		return result, nil
	}
	command, path, ok := fsReadLikeCommand(arguments)
	if !ok {
		return result, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return result, nil
	}
	if _, hasError := payload["error"]; hasError {
		return result, nil
	}
	rawContent, ok := payload["content"]
	if !ok {
		return result, nil
	}
	var content string
	if err := json.Unmarshal(rawContent, &content); err != nil || content == "" {
		return result, nil
	}

	ref, info := e.registerContent(content, map[string]any{
		"tool":    toolName,
		"command": command,
		"path":    path,
	})
	payload["contentRef"] = mustJSONRaw(ref)
	payload["contentChars"] = mustJSONRaw(len([]rune(content)))
	payload["contentHash"] = mustJSONRaw(info["contentHash"])
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode tool result content reference: %w", err)
	}
	return string(data), nil
}

func (e *Executor) resolve(functionName string) (resolvedRPC, error) {
	for _, desc := range e.descriptors {
		if desc.GetAddress() == "" {
			continue
		}
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

func invokeServiceFunction(ctx context.Context, conn *grpc.ClientConn, method string, in *dynamicpb.Message, response protoreflect.MessageDescriptor, rpc *servicev1.RpcDescriptor) (*dynamicpb.Message, error) {
	attempts := serviceFunctionMaxAttempts(rpc)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		out := dynamicpb.NewMessage(response)
		err := conn.Invoke(ctx, method, in, out)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if attempt == attempts || !serviceFunctionRetryable(rpc, err) {
			return nil, err
		}
		if err := waitServiceFunctionRetry(ctx, rpc, attempt); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
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
	code := normalizeRetryCode(status.Code(err).String())
	for _, retryable := range rpc.GetRetryPolicy().GetRetryableCodes() {
		if normalizeRetryCode(retryable) == code {
			return true
		}
	}
	return false
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
			schema["description"] = description + " For input, prefer inputRef or contentRef returned from fs read or document_ExtractText results when embedding source files; otherwise provide explicit input."
		}
		properties["inputRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by fs read or document_ExtractText. Prefer this over copying source text into input.",
		}
		properties["contentRef"] = map[string]any{
			"type":        "string",
			"description": "Alias for inputRef. Reference returned by fs read or document_ExtractText.",
		}
	case "quark.indexer.v1.IndexRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " For textContent, prefer textContentRef returned from fs read or document_ExtractText results when indexing source files; otherwise provide explicit textContent."
		}
		properties["embeddingRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by embedding_Embed. Prefer this over copying embedding vectors manually.",
		}
		properties["textContentRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by fs read or document_ExtractText. Prefer this over copying source text into textContent.",
		}
	case "quark.indexer.v1.UpsertChunkRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " For textContent, prefer textContentRef returned from fs read or document_ExtractText results when indexing source files; otherwise provide explicit textContent."
		}
		properties["embeddingRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by embedding_Embed. Prefer this over copying embedding vectors manually.",
		}
		properties["textContentRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by fs read or document_ExtractText. Prefer this over copying source text into textContent.",
		}
	case "quark.indexer.v1.QueryRequest":
		properties["queryVectorRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by embedding_Embed for the user's query. Prefer this over copying query vectors manually.",
		}
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
		return []string{"chunkId", "embeddingRef"}
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

func (e *Executor) documentExtractTextToolResult(msg protoreflect.ProtoMessage) (string, error) {
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
	ref, info := e.registerContent(text, map[string]any{
		"serviceFunction": "document_ExtractText",
		"sourceHash":      sourceHash,
	})
	payload["contentRef"] = mustJSONRaw(ref)
	payload["contentChars"] = mustJSONRaw(len([]rune(text)))
	payload["contentHash"] = mustJSONRaw(info["contentHash"])
	if sourceHash != "" {
		payload["sourceHash"] = mustJSONRaw(sourceHash)
	}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode document text content reference: %w", err)
	}
	return string(out), nil
}

func (e *Executor) expandRuntimeReferences(typeName, arguments string) (string, error) {
	if strings.TrimSpace(arguments) == "" {
		return arguments, nil
	}
	switch typeName {
	case "quark.embedding.v1.EmbedRequest":
		expanded, err := e.expandContentReference(arguments, "inputRef", "input")
		if err != nil {
			return "", err
		}
		return e.expandContentReference(expanded, "contentRef", "input")
	case "quark.indexer.v1.IndexRequest":
		expanded, err := e.expandVectorReference(arguments, "embeddingRef", "embedding")
		if err != nil {
			return "", err
		}
		return e.expandContentReference(expanded, "textContentRef", "textContent")
	case "quark.indexer.v1.UpsertChunkRequest":
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
		return "", fmt.Errorf("%s %q was not produced by an fs read or document_ExtractText call in this runtime session", refField, ref)
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

func fsReadLikeCommand(arguments string) (command, path string, ok bool) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", "", false
	}
	if rawCommand, exists := payload["command"]; exists {
		_ = json.Unmarshal(rawCommand, &command)
	}
	command = strings.TrimSpace(command)
	if command != "read" {
		return "", "", false
	}
	if rawPath, exists := payload["path"]; exists {
		_ = json.Unmarshal(rawPath, &path)
	}
	return command, strings.TrimSpace(path), true
}

func (e *Executor) registerContent(content string, metadata map[string]any) (string, map[string]any) {
	sum := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(sum[:])
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextContent++
	ref := fmt.Sprintf("content_%d", e.nextContent)
	info := cloneMetadata(metadata)
	info["contentHash"] = contentHash
	info["chars"] = len([]rune(content))
	e.contents[ref] = content
	e.contents[contentHash] = content
	e.contentInfo[ref] = cloneMetadata(info)
	e.contentInfo[contentHash] = cloneMetadata(info)
	return ref, cloneMetadata(info)
}

func (e *Executor) contentByRef(ref string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	content, ok := e.contents[ref]
	return content, ok
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
	e.mu.RLock()
	defer e.mu.RUnlock()
	vector, ok := e.embeddings[ref]
	if !ok {
		return nil, false
	}
	return cloneVector(vector), true
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
