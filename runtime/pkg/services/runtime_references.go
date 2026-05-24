package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (e *Executor) expandRuntimeReferences(typeName, arguments string) (string, error) {
	if strings.TrimSpace(arguments) == "" {
		return arguments, nil
	}
	switch typeName {
	case "quark.gateway.v1.EmbedRequest":
		expanded, err := e.expandGatewayEmbeddingInputs(arguments)
		if err != nil {
			return "", err
		}
		return stripEmbeddingRequestOverrides(expanded), nil
	case "quark.gateway.v1.GenerateRequest", "quark.gateway.v1.StreamGenerateRequest":
		return e.expandGatewayMessageReferences(arguments)
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

func (e *Executor) expandGatewayEmbeddingInputs(arguments string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode service arguments: %w", err)
	}
	for _, alias := range []struct {
		Field string
		Kind  string
	}{
		{Field: "inputRef", Kind: "CONTENT_KIND_CONTENT_REF"},
		{Field: "contentRef", Kind: "CONTENT_KIND_CONTENT_REF"},
		{Field: "pageRef", Kind: "CONTENT_KIND_PAGE_REF"},
		{Field: "imageRef", Kind: "CONTENT_KIND_IMAGE_REF"},
		{Field: "mediaRef", Kind: "CONTENT_KIND_IMAGE_REF"},
		{Field: "artifactRef", Kind: "CONTENT_KIND_ARTIFACT_REF"},
	} {
		raw, ok := payload[alias.Field]
		if !ok {
			continue
		}
		ref, ok := singleStringArgument(raw)
		if !ok {
			return "", fmt.Errorf("%s must be a non-empty reference string", alias.Field)
		}
		payload["inputs"] = mustJSONRaw([]map[string]any{{
			"content": []map[string]any{{"kind": alias.Kind, "ref": ref}},
		}})
		delete(payload, alias.Field)
		break
	}
	rawInputs, ok := payload["inputs"]
	if !ok {
		return arguments, nil
	}
	var inputs []map[string]json.RawMessage
	if err := json.Unmarshal(rawInputs, &inputs); err != nil {
		return "", fmt.Errorf("inputs must be an array: %w", err)
	}
	for i, input := range inputs {
		rawContent, ok := input["content"]
		if !ok {
			continue
		}
		resolved, err := e.resolveGatewayContentParts(rawContent, fmt.Sprintf("inputs[%d]", i))
		if err != nil {
			return "", err
		}
		input["content"] = resolved
	}
	payload["inputs"] = mustJSONRaw(inputs)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode Gateway embedding inputs: %w", err)
	}
	return string(data), nil
}

func (e *Executor) expandGatewayMessageReferences(arguments string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode service arguments: %w", err)
	}
	rawMessages, ok := payload["messages"]
	if !ok {
		return arguments, nil
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		return "", fmt.Errorf("messages must be an array: %w", err)
	}
	for i, message := range messages {
		rawContent, ok := message["content"]
		if !ok {
			continue
		}
		resolved, err := e.resolveGatewayContentParts(rawContent, fmt.Sprintf("messages[%d]", i))
		if err != nil {
			return "", err
		}
		message["content"] = resolved
	}
	payload["messages"] = mustJSONRaw(messages)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode Gateway messages: %w", err)
	}
	return string(data), nil
}

func (e *Executor) resolveGatewayContentParts(raw json.RawMessage, parent string) (json.RawMessage, error) {
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("%s.content must be an array: %w", parent, err)
	}
	for i, part := range parts {
		kind := rawStringArgument(part, "kind")
		ref := rawStringArgument(part, "ref")
		switch kind {
		case "CONTENT_KIND_CONTENT_REF", "CONTENT_KIND_PAGE_REF":
			content, ok := e.contentByRef(ref)
			if !ok {
				return nil, fmt.Errorf("%s.content[%d] reference %q was not produced by an io_Read or document_ExtractText call in this runtime session", parent, i, ref)
			}
			parts[i] = map[string]json.RawMessage{
				"kind": mustJSONRaw("CONTENT_KIND_TEXT"),
				"text": mustJSONRaw(content),
			}
		case "CONTENT_KIND_IMAGE_REF", "CONTENT_KIND_ARTIFACT_REF", "CONTENT_KIND_FILE_REF":
			content, metadata, ok := e.mediaByRef(ref)
			if !ok {
				return nil, fmt.Errorf("%s.content[%d] media reference %q was not produced by io_ReadMedia or document_ExtractImages in this runtime session", parent, i, ref)
			}
			parts[i] = map[string]json.RawMessage{
				"kind":      mustJSONRaw("CONTENT_KIND_IMAGE_DATA"),
				"imageData": mustJSONRaw(content),
				"mimeType":  mustJSONRaw(metadataString(metadata, "mimeType", "mime_type")),
			}
		}
	}
	return mustJSONRaw(parts), nil
}

func stripEmbeddingRequestOverrides(arguments string) string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return arguments
	}
	delete(payload, "model")
	delete(payload, "dimensions")
	delete(payload, "provider")
	delete(payload, "options")
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
		return fmt.Errorf("%s requires %s from gateway_Embed; direct %s values are not accepted in runtime tool calls", typeName, refField, directField)
	}
	return fmt.Errorf("%s requires %s from gateway_Embed", typeName, refField)
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

func (e *Executor) expandContentReferenceList(arguments, refField, contentField string) (string, error) {
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
	rawContent, err := json.Marshal([]string{content})
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
		return "", fmt.Errorf("%s %q was not produced by gateway_Embed in this runtime session", refField, ref)
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
