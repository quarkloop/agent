package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

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
