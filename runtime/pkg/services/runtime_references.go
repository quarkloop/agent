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
		arguments = e.promoteContentReference(arguments, "input", "contentRef")
		expanded, err := e.expandContentReferenceList(arguments, "inputRef", "input")
		if err != nil {
			return "", err
		}
		expanded, err = e.expandContentReferenceList(expanded, "contentRef", "input")
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
