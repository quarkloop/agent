package services

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/quarkloop/runtime/pkg/runcontext"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

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
	spaceID := strings.TrimSpace(runcontext.SpaceID(ctx))
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
		case field.IsList() && field.Kind() == protoreflect.StringKind:
			normalized, err := normalizeStringListField(raw)
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

func normalizeStringListField(raw json.RawMessage) (json.RawMessage, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		item, err := normalizeStringField(value)
		if err != nil {
			return nil, err
		}
		var text string
		if err := json.Unmarshal(item, &text); err != nil {
			return nil, err
		}
		normalized = append(normalized, text)
	}
	return json.Marshal(normalized)
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
	raw = unwrapJSONEncodedCollection(raw)
	var rawItems []json.RawMessage
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return raw, err
	}
	items := make([]map[string]json.RawMessage, 0, len(rawItems))
	for _, rawItem := range rawItems {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(unwrapJSONEncodedObject(rawItem), &item); err != nil {
			return raw, err
		}
		if err := normalizeStringMapMessage(desc, item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	data, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// unwrapJSONEncodedCollection accepts a collection encoded once as a JSON
// string by a model tool call. It does not coerce arbitrary strings: the
// normal collection decoder remains authoritative for validation.
func unwrapJSONEncodedCollection(raw json.RawMessage) json.RawMessage {
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return raw
	}
	encoded = strings.TrimSpace(encoded)
	if !strings.HasPrefix(encoded, "[") || !strings.HasSuffix(encoded, "]") {
		return raw
	}
	return json.RawMessage(encoded)
}

// unwrapJSONEncodedObject accepts an individual structured item encoded once
// as a JSON string by a model tool call. The message decoder still validates
// the resulting object against the protobuf contract.
func unwrapJSONEncodedObject(raw json.RawMessage) json.RawMessage {
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return raw
	}
	encoded = strings.TrimSpace(encoded)
	if !strings.HasPrefix(encoded, "{") || !strings.HasSuffix(encoded, "}") {
		return raw
	}
	return json.RawMessage(encoded)
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
