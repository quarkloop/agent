package services

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

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
	case "quark.gateway.v1.EmbedRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " For source material, prefer contentRef/pageRef/imageRef returned from IO or document results. For explicit public images or mixed input use typed inputs[].content[]. Provider, model, dimensions, and options are controlled by Gateway configuration."
		}
		delete(properties, "provider")
		delete(properties, "model")
		delete(properties, "dimensions")
		delete(properties, "options")
		properties["inputRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by io_Read or document_ExtractText. Prefer this over copying source text into inputs.",
		}
		properties["contentRef"] = map[string]any{
			"type":        "string",
			"description": "Alias for inputRef. Reference returned by io_Read or document_ExtractText.",
		}
		properties["pageRef"] = map[string]any{
			"type":        "string",
			"description": "Page text reference returned by document_ExtractText.",
		}
		properties["imageRef"] = map[string]any{
			"type":        "string",
			"description": "Image/media reference returned by io_ReadMedia or document_ExtractImages.",
		}
		removeGatewayInlineImageBytes(properties)
	case "quark.gateway.v1.GenerateRequest", "quark.gateway.v1.StreamGenerateRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " Use typed message content and opaque runtime references for extracted text/pages/media. Do not inline binary media bytes."
		}
		removeGatewayMessageInlineImageBytes(properties)
	case "quark.indexer.v1.IndexRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " Runtime tool calls must use embeddingRef returned from gateway_Embed; direct embedding vectors are not accepted. For textContent, prefer textContentRef returned from io_Read or document_ExtractText results when indexing source files; otherwise provide explicit textContent."
		}
		delete(properties, "embedding")
		properties["embeddingRef"] = map[string]any{
			"type":        "string",
			"description": "Required reference returned by gateway_Embed. Do not copy embedding vectors manually.",
		}
		properties["textContentRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by io_Read or document_ExtractText. Prefer this over copying source text into textContent.",
		}
	case "quark.indexer.v1.UpsertChunkRequest":
		if description, ok := schema["description"].(string); ok {
			schema["description"] = description + " Runtime tool calls must use embeddingRef returned from gateway_Embed; direct embedding vectors are not accepted. For textContent, prefer textContentRef returned from io_Read or document_ExtractText results when indexing source files; otherwise provide explicit textContent. For document indexing, provide a complete canonical knowledge record: document, sourceMetadata, provenance, facts, entities, relations, and citations. Use an empty relations array only when no supported relation exists."
		}
		delete(properties, "embedding")
		applyCanonicalUpsertChunkPropertyDescriptions(properties)
		properties["embeddingRef"] = map[string]any{
			"type":        "string",
			"description": "Required reference returned by gateway_Embed. Do not copy embedding vectors manually.",
		}
		properties["textContentRef"] = map[string]any{
			"type":        "string",
			"description": "Reference returned by io_Read or document_ExtractText. Prefer this over copying source text into textContent.",
		}
	case "quark.indexer.v1.QueryRequest":
		delete(properties, "queryVector")
		properties["queryVectorRef"] = map[string]any{
			"type":        "string",
			"description": "Required reference returned by gateway_Embed for the user's query. Do not copy query vectors manually.",
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

func removeGatewayInlineImageBytes(properties map[string]any) {
	inputs, ok := properties["inputs"].(map[string]any)
	if !ok {
		return
	}
	items, _ := inputs["items"].(map[string]any)
	inputProps, _ := items["properties"].(map[string]any)
	content, _ := inputProps["content"].(map[string]any)
	contentItems, _ := content["items"].(map[string]any)
	contentProps, _ := contentItems["properties"].(map[string]any)
	delete(contentProps, "imageData")
}

func removeGatewayMessageInlineImageBytes(properties map[string]any) {
	messages, ok := properties["messages"].(map[string]any)
	if !ok {
		return
	}
	items, _ := messages["items"].(map[string]any)
	messageProps, _ := items["properties"].(map[string]any)
	content, _ := messageProps["content"].(map[string]any)
	contentItems, _ := content["items"].(map[string]any)
	contentProps, _ := contentItems["properties"].(map[string]any)
	delete(contentProps, "imageData")
}

func applyCanonicalUpsertChunkPropertyDescriptions(properties map[string]any) {
	describeObjectProperty(properties, "document", "Required source document identity with stable id, filename/name, type, sourceUri, and useful document metadata.")
	describeObjectProperty(properties, "sourceMetadata", "Required source metadata map with filename, documentId, documentName, documentType, sourceUri, sourceHash when known, and extraction/classification hints.", "minProperties", 1)
	describeObjectProperty(properties, "provenance", "Required provenance for the original source and producing agent/tool trace, including sourceUri, sourceHash when known, producedBy, ingestedAt or traceId when available.")
	describeArrayProperty(properties, "facts", "Required evidence-backed facts extracted by the agent from the source. Include subject, predicate, object, confidence, and citations for source-backed facts.", 1)
	describeArrayProperty(properties, "entities", "Required normalized people, organizations, documents, products, topics, dates, or other entities useful for retrieval and graph traversal.", 1)
	describeArrayProperty(properties, "relations", "Required relation array. Include supported relations between normalized entity IDs, or an empty array when no relation is supported by the source.", 0)
	describeArrayProperty(properties, "citations", "Required source evidence spans for the chunk or extracted facts, with sourceUri, textSpan, offsets when known, and confidence.", 1)
	describeObjectProperty(properties, "embeddingMetadata", "Embedding metadata returned by or derived from gateway_Embed, including provider, model, dimensions, and contentHash when known.")
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
	case "quark.gateway.v1.EmbedRequest":
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
