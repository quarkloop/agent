package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/quarkloop/runtime/pkg/sourceid"
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
	case "quark.indexer.v1.UpsertChunkRequest":
		arguments = e.promoteContentReference(arguments, "textContent", "textContentRef")
		if err := e.validateChunkEmbeddingSource(arguments); err != nil {
			return "", err
		}
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

func (e *Executor) normalizeGatewayEmbeddingReferenceProvenance(arguments string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode gateway_Embed arguments: %w", err)
	}
	shorthandExpanded := false
	if rawText, ok := payload["text"]; ok {
		for _, key := range []string{"inputs", "inputRef", "contentRef", "pageRef", "pageRefs", "imageRef"} {
			if raw, exists := payload[key]; exists && string(raw) != "null" {
				return "", fmt.Errorf("gateway_Embed must provide text or %s, not both", key)
			}
		}
		text, ok := singleStringArgument(rawText)
		if !ok {
			return "", fmt.Errorf("gateway_Embed text must be a non-empty literal string")
		}
		payload["inputs"] = mustJSONRaw([]map[string]any{{
			"content": []map[string]any{{
				"kind": "CONTENT_KIND_TEXT",
				"text": text,
			}},
		}})
		delete(payload, "text")
		shorthandExpanded = true
	}
	if rawPageRefs, ok := payload["pageRefs"]; ok {
		if _, hasInputs := payload["inputs"]; hasInputs {
			return "", fmt.Errorf("gateway_Embed must provide either pageRefs or inputs, not both")
		}
		var refs []string
		if err := json.Unmarshal(rawPageRefs, &refs); err != nil || len(refs) == 0 {
			return "", fmt.Errorf("pageRefs must be a non-empty array of references exactly returned by document extraction")
		}
		inputs := make([]map[string]any, 0, len(refs))
		for _, ref := range refs {
			if strings.TrimSpace(ref) == "" {
				return "", fmt.Errorf("pageRefs must contain non-empty reference strings")
			}
			inputs = append(inputs, map[string]any{
				"content": []map[string]any{{"kind": "CONTENT_KIND_PAGE_REF", "ref": strings.TrimSpace(ref)}},
			})
		}
		payload["inputs"] = mustJSONRaw(inputs)
		delete(payload, "pageRefs")
		shorthandExpanded = true
	}
	if rawPageRef, ok := payload["pageRef"]; ok {
		if _, hasInputs := payload["inputs"]; hasInputs {
			return "", fmt.Errorf("gateway_Embed must provide either pageRef or inputs, not both")
		}
		ref, ok := singleStringArgument(rawPageRef)
		if !ok {
			return "", fmt.Errorf("pageRef must be a non-empty reference string")
		}
		payload["inputs"] = mustJSONRaw([]map[string]any{{
			"content": []map[string]any{{"kind": "CONTENT_KIND_PAGE_REF", "ref": ref}},
		}})
		delete(payload, "pageRef")
		shorthandExpanded = true
	}
	rawInputs, ok := payload["inputs"]
	if !ok {
		return arguments, nil
	}
	var inputs []map[string]json.RawMessage
	inputsEncoded, err := decodeStructuredToolArgument(rawInputs, &inputs)
	if err != nil {
		return "", fmt.Errorf("gateway_Embed inputs must be an array: %w", err)
	}
	changed := shorthandExpanded || inputsEncoded
	for i, input := range inputs {
		rawContent, ok := input["content"]
		if !ok {
			continue
		}
		var parts []map[string]json.RawMessage
		contentEncoded, err := decodeStructuredToolArgument(rawContent, &parts)
		if err != nil {
			return "", fmt.Errorf("gateway_Embed inputs[%d].content must be an array: %w", i, err)
		}
		if contentEncoded {
			input["content"] = mustJSONRaw(parts)
			changed = true
		}
		sourceURI, hasPageRef, err := e.sourceURIForPageReferenceInput(input, parts, i)
		if err != nil {
			return "", err
		}
		if !hasPageRef {
			continue
		}
		var metadata map[string]string
		if rawMetadata, ok := input["metadata"]; ok {
			metadataEncoded, err := decodeStructuredToolArgument(rawMetadata, &metadata)
			if err != nil {
				return "", fmt.Errorf("gateway_Embed inputs[%d].metadata must contain strings: %w", i, err)
			}
			if metadataEncoded {
				input["metadata"] = mustJSONRaw(metadata)
				changed = true
			}
		}
		if metadata == nil {
			metadata = make(map[string]string)
		}
		metadata["sourceUri"] = sourceURI
		input["metadata"] = mustJSONRaw(metadata)
		changed = true
	}
	if !changed {
		return arguments, nil
	}
	payload["inputs"] = mustJSONRaw(inputs)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode gateway_Embed page reference provenance: %w", err)
	}
	return string(data), nil
}

func decodeStructuredToolArgument(raw json.RawMessage, target any) (bool, error) {
	if err := json.Unmarshal(raw, target); err == nil {
		return false, nil
	} else {
		var encoded string
		if stringErr := json.Unmarshal(raw, &encoded); stringErr != nil {
			return false, err
		}
		if encodedErr := json.Unmarshal([]byte(encoded), target); encodedErr != nil {
			return false, err
		}
		return true, nil
	}
}

func (e *Executor) sourceURIForPageReferenceInput(input map[string]json.RawMessage, parts []map[string]json.RawMessage, inputIndex int) (string, bool, error) {
	var metadata map[string]string
	if rawMetadata, ok := input["metadata"]; ok {
		if err := json.Unmarshal(rawMetadata, &metadata); err != nil {
			return "", false, fmt.Errorf("gateway_Embed inputs[%d].metadata must contain strings: %w", inputIndex, err)
		}
	}
	suppliedSourceURI := firstNonEmptyString(metadata["sourceUri"], metadata["source_uri"])
	sourceURI := suppliedSourceURI
	hasPageRef := false
	for partIndex, part := range parts {
		if rawStringArgument(part, "kind") != "CONTENT_KIND_PAGE_REF" {
			continue
		}
		hasPageRef = true
		ref := rawStringArgument(part, "ref")
		if ref == "" {
			return "", false, fmt.Errorf("gateway_Embed inputs[%d].content[%d] page reference is empty", inputIndex, partIndex)
		}
		referenceMetadata, ok := e.contentMetadataByRef(ref)
		if !ok {
			return "", false, fmt.Errorf("gateway_Embed inputs[%d].content[%d] page reference %q was not issued by document extraction in this runtime session; use one of %s", inputIndex, partIndex, ref, e.availablePageReferenceSummary())
		}
		referenceSourceURI := metadataString(referenceMetadata, "sourceURI", "sourceUri", "source_uri", "path")
		if referenceSourceURI == "" {
			return "", false, fmt.Errorf("gateway_Embed inputs[%d].content[%d] page reference %q has no source provenance", inputIndex, partIndex, ref)
		}
		if sourceURI != "" && !sourceid.Equal(sourceURI, referenceSourceURI) {
			return "", false, fmt.Errorf("gateway_Embed inputs[%d] sourceUri %q does not match page reference sourceUri %q", inputIndex, sourceURI, referenceSourceURI)
		}
		sourceURI = referenceSourceURI
	}
	return sourceURI, hasPageRef, nil
}

func (e *Executor) availablePageReferenceSummary() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	type candidate struct {
		ref    string
		source string
	}
	var candidates []candidate
	for ref, metadata := range e.contentInfo {
		if _, ok := metadata["pageNumber"]; !ok {
			continue
		}
		source := metadataString(metadata, "sourceURI", "sourceUri", "source_uri", "path")
		candidates = append(candidates, candidate{ref: ref, source: source})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ref < candidates[j].ref })
	if len(candidates) == 0 {
		return "the pageRef values returned by document extraction"
	}
	const maxReferences = 12
	if len(candidates) > maxReferences {
		candidates = candidates[:maxReferences]
	}
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.source == "" {
			parts = append(parts, candidate.ref)
			continue
		}
		parts = append(parts, candidate.ref+" ("+candidate.source+")")
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (e *Executor) expandGatewayEmbeddingInputs(arguments string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", fmt.Errorf("decode service arguments: %w", err)
	}
	if rawRefs, ok := payload["pageRefs"]; ok {
		var refs []string
		if err := json.Unmarshal(rawRefs, &refs); err != nil || len(refs) == 0 {
			return "", fmt.Errorf("pageRefs must be a non-empty array of reference strings")
		}
		inputs := make([]map[string]any, 0, len(refs))
		for _, ref := range refs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				return "", fmt.Errorf("pageRefs must contain non-empty reference strings")
			}
			inputs = append(inputs, map[string]any{
				"content": []map[string]any{{"kind": "CONTENT_KIND_PAGE_REF", "ref": ref}},
			})
		}
		payload["inputs"] = mustJSONRaw(inputs)
		delete(payload, "pageRefs")
	}
	for _, alias := range []struct {
		Field string
		Kind  string
	}{
		{Field: "contentRef", Kind: "CONTENT_KIND_CONTENT_REF"},
		{Field: "pageRef", Kind: "CONTENT_KIND_PAGE_REF"},
		{Field: "inputRef", Kind: "CONTENT_KIND_CONTENT_REF"},
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
		for _, field := range []string{"contentRef", "pageRef", "pageRefs", "inputRef", "imageRef", "mediaRef", "artifactRef"} {
			delete(payload, field)
		}
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
	case "quark.indexer.v1.UpsertChunkRequest":
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

func (e *Executor) validateChunkEmbeddingSource(arguments string) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return fmt.Errorf("decode service arguments: %w", err)
	}
	embeddingRef := rawStringArgument(payload, "embeddingRef")
	if embeddingRef == "" {
		return nil
	}
	expectedSourceURI, hasExpectedSourceURI := e.embeddingSourceURIByRef(embeddingRef)
	if hasExpectedSourceURI && expectedSourceURI != "" {
		indexedSourceURI := firstPayloadString(payload, nil, []string{"sourceUri", "source_uri"}, "document", "provenance", "sourceMetadata")
		switch {
		case indexedSourceURI == "":
			return fmt.Errorf("embeddingRef %q requires the indexed chunk to preserve sourceUri %q", embeddingRef, expectedSourceURI)
		case !sourceid.Equal(indexedSourceURI, expectedSourceURI):
			return fmt.Errorf("embeddingRef %q belongs to sourceUri %q, not indexed sourceUri %q", embeddingRef, expectedSourceURI, indexedSourceURI)
		}
	}
	sourceHash, ok := e.embeddingSourceContentHash(embeddingRef)
	if !ok || sourceHash == "" {
		return nil
	}
	textRef := rawStringArgument(payload, "textContentRef")
	text := rawStringArgument(payload, "textContent")
	if textRef != "" {
		var found bool
		text, found = e.contentByRef(textRef)
		if !found {
			return fmt.Errorf("textContentRef %q was not produced by an io_Read or document_ExtractText call in this runtime session", textRef)
		}
	}
	if text == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(text))
	if hex.EncodeToString(sum[:]) != sourceHash {
		return fmt.Errorf("embeddingRef %q was generated for different content than the chunk text; create an embedding for this indexed content", embeddingRef)
	}
	return nil
}

func (e *Executor) embeddingSourceContentHash(ref string) (string, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	hash, ok := e.embeddingSourceHash[ref]
	return hash, ok
}

func (e *Executor) embeddingSourceURIByRef(ref string) (string, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	uri, ok := e.embeddingSourceURI[ref]
	return uri, ok
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
