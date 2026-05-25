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
	switch typeName {
	case "quark.gateway.v1.EmbedRequest":
		delete(properties, "provider")
		delete(properties, "model")
		delete(properties, "dimensions")
		delete(properties, "options")
		properties["inputRef"] = map[string]any{"type": "string"}
		properties["contentRef"] = map[string]any{"type": "string"}
		properties["pageRef"] = map[string]any{"type": "string"}
		properties["imageRef"] = map[string]any{"type": "string"}
		removeGatewayInlineImageBytes(properties)
	case "quark.gateway.v1.GenerateRequest", "quark.gateway.v1.StreamGenerateRequest":
		removeGatewayMessageInlineImageBytes(properties)
	case "quark.indexer.v1.UpsertChunkRequest":
		delete(properties, "embedding")
		applyCanonicalUpsertChunkConstraints(properties)
		properties["embeddingRef"] = map[string]any{"type": "string"}
		properties["textContentRef"] = map[string]any{"type": "string"}
	case "quark.indexer.v1.QueryRequest":
		delete(properties, "queryVector")
		properties["queryVectorRef"] = map[string]any{"type": "string"}
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
