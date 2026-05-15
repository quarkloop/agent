package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/quarkloop/pkg/plugin"
	buildreleasev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/buildrelease/v1"
	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
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
			name := ToolNameFor(desc.GetName(), rpc.GetMethod())
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
	arguments, err = e.expandRuntimeReferences(rpc.GetRequest(), arguments)
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
	out := dynamicpb.NewMessage(respType.Descriptor())

	conn, err := servicekit.Dial(ctx, resolved.address)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", resolved.address, err)
	}
	defer conn.Close()

	fullMethod := "/" + rpc.GetService() + "/" + rpc.GetMethod()
	if err := conn.Invoke(ctx, fullMethod, in, out); err != nil {
		return "", fmt.Errorf("call %s: %w", fullMethod, err)
	}
	if rpc.GetResponse() == "quark.embedding.v1.EmbedResponse" {
		return e.embeddingToolResult(out)
	}
	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encode response: %w", err)
	}
	return string(data), nil
}

func (e *Executor) resolve(functionName string) (resolvedRPC, error) {
	for _, desc := range e.descriptors {
		if desc.GetAddress() == "" {
			continue
		}
		for _, rpc := range desc.GetRpcs() {
			if ToolNameFor(desc.GetName(), rpc.GetMethod()) != functionName {
				continue
			}
			return resolvedRPC{rpc: rpc, address: desc.GetAddress()}, nil
		}
	}
	return resolvedRPC{}, fmt.Errorf("service function not found: %q", functionName)
}

var serviceToolUnsafeChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

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
	case "quark.indexer.v1.IndexRequest":
		properties["embeddingRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by embedding_Embed. Prefer this over copying embedding vectors manually.",
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
		return []string{"input"}
	case "quark.indexer.v1.IndexRequest":
		return []string{"chunkId", "textContent", "embeddingRef"}
	case "quark.indexer.v1.QueryRequest":
		return []string{"queryVectorRef"}
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

func (e *Executor) expandRuntimeReferences(typeName, arguments string) (string, error) {
	if strings.TrimSpace(arguments) == "" {
		return arguments, nil
	}
	switch typeName {
	case "quark.indexer.v1.IndexRequest":
		return e.expandVectorReference(arguments, "embeddingRef", "embedding")
	case "quark.indexer.v1.QueryRequest":
		return e.expandVectorReference(arguments, "queryVectorRef", "queryVector")
	default:
		return arguments, nil
	}
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
