package services

import (
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func requestParameters(typeName string) map[string]any {
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(typeName))
	if err != nil {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	}

	schema := messageJSONSchema(msgType.Descriptor(), 0)
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
	if isDocumentInputRequest(typeName) {
		applyAgentDocumentSourceConstraints(schema, properties)
		return
	}
	switch typeName {
	case "quark.gateway.v1.EmbedRequest":
		delete(properties, "provider")
		delete(properties, "model")
		delete(properties, "dimensions")
		delete(properties, "options")
		properties["inputs"].(map[string]any)["description"] = "Typed embedding inputs for text or media. For a user question about indexed knowledge, use exactly one non-empty inline text retrieval query representing that request and no reference shortcut. For extracted PDF pages used during indexing, use pageRefs instead."
		properties["text"] = map[string]any{"type": "string", "description": "One literal text query when the active workflow exposes a text-only retrieval embedding."}
		properties["inputRef"] = map[string]any{"type": "string", "description": "One runtime-issued text content reference from earlier tool output; never a user-question label."}
		properties["contentRef"] = map[string]any{"type": "string", "description": "One runtime-issued text content reference from earlier tool output; never a user-question label."}
		properties["pageRef"] = map[string]any{"type": "string", "description": "One pageRef exactly as returned by document extraction."}
		properties["pageRefs"] = map[string]any{
			"type":        "array",
			"minItems":    1,
			"items":       map[string]any{"type": "string"},
			"description": "Page references exactly as returned by document extraction; use one pageRef per PDF source when embedding a document batch. Set pageRefs alone and omit inputs.",
		}
		properties["imageRef"] = map[string]any{"type": "string", "description": "One runtime-issued image reference."}
		removeGatewayInlineImageBytes(properties)
	case "quark.gateway.v1.GenerateRequest", "quark.gateway.v1.StreamGenerateRequest":
		removeGatewayMessageInlineImageBytes(properties)
	case "quark.indexer.v1.UpsertChunkRequest":
		delete(properties, "embedding")
		applyCanonicalUpsertChunkConstraints(properties)
		properties["embeddingRef"] = map[string]any{"type": "string"}
		properties["textContentRef"] = map[string]any{"type": "string"}
	case "quark.indexer.v1.UpsertDocumentRequest":
		applyCanonicalUpsertDocumentConstraints(properties)
	case "quark.indexer.v1.QueryRequest":
		delete(properties, "queryVector")
		properties["queryVectorRef"] = map[string]any{"type": "string"}
	case "quark.citation.v1.VerifyGroundingRequest", "quark.citation.v1.ScoreCoverageRequest":
		applyGroundedClaimConstraints(properties)
	case "quark.citation.v1.RenderReferencesRequest":
		applyCitationListConstraints(properties, "citations", "Normalized source spans to render. Each span must include sourceUri and should include textSpan and offsets when retrieved evidence provides them.", false)
	}
}

func applyAgentDocumentSourceConstraints(schema map[string]any, properties map[string]any) {
	input, ok := properties["input"].(map[string]any)
	if !ok {
		return
	}
	inputProperties, ok := input["properties"].(map[string]any)
	if !ok {
		return
	}
	// Agents identify user-approved documents by source URI. Inline bytes are
	// a programmatic service input and must not be copied through LLM calls.
	delete(inputProperties, "content")
	delete(inputProperties, "contentRef")
	input["required"] = []string{"sourceUri"}
	input["description"] = "A user-approved local document source. Provide input.sourceUri exactly as discovered; do not invent artifact or content references."
	schema["required"] = []string{"input"}
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

func applyCanonicalUpsertChunkConstraints(properties map[string]any) {
	if sourceMetadata, ok := properties["sourceMetadata"].(map[string]any); ok {
		sourceMetadata["minProperties"] = 1
	}
	for _, name := range []string{"facts", "entities", "citations"} {
		if values, ok := properties[name].(map[string]any); ok {
			values["minItems"] = 1
		}
	}
}

func applyCanonicalUpsertDocumentConstraints(properties map[string]any) {
	document, ok := properties["document"].(map[string]any)
	if !ok {
		return
	}
	document["required"] = []string{"id", "sourceUri"}
}

func applyGroundedClaimConstraints(properties map[string]any) {
	claims, ok := properties["claims"].(map[string]any)
	if !ok {
		return
	}
	claims["minItems"] = 1
	claims["description"] = "Array of claims selected from retrieved evidence; do not JSON-encode this array as a string."
	items, _ := claims["items"].(map[string]any)
	if items == nil {
		return
	}
	items["required"] = []string{"id", "claim", "citations"}
	claimProperties, _ := items["properties"].(map[string]any)
	applyCitationListConstraints(claimProperties, "citations", "Evidence spans supporting this claim. textSpan is required for mechanical grounding verification and must be copied from retrieved evidence.", true)
}

func applyCitationListConstraints(properties map[string]any, fieldName, description string, requireTextSpan bool) {
	citations, ok := properties[fieldName].(map[string]any)
	if !ok {
		return
	}
	citations["minItems"] = 1
	citations["description"] = description
	items, _ := citations["items"].(map[string]any)
	if items == nil {
		return
	}
	required := []string{"id", "sourceUri"}
	if requireTextSpan {
		required = append(required, "textSpan")
	}
	items["required"] = required
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
	case "quark.indexer.v1.UpsertDocumentRequest":
		return []string{"document"}
	case "quark.indexer.v1.QueryRequest":
		return []string{"queryVectorRef"}
	case "quark.indexer.v1.DeleteDocumentRequest":
		return []string{"documentId"}
	case "quark.indexer.v1.DeleteChunkRequest":
		return []string{"chunkId"}
	case "quark.citation.v1.VerifyGroundingRequest", "quark.citation.v1.ScoreCoverageRequest":
		return []string{"claims"}
	case "quark.citation.v1.RenderReferencesRequest":
		return []string{"citations"}
	default:
		return nil
	}
}
