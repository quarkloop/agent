package services

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (e *Executor) embeddingToolResult(msg protoreflect.ProtoMessage) (string, error) {
	reflected := msg.ProtoReflect()
	embeddingsField := reflected.Descriptor().Fields().ByName("embeddings")
	if embeddingsField == nil || !embeddingsField.IsList() {
		return "", fmt.Errorf("gateway embedding response descriptor is missing embeddings")
	}

	embeddings := reflected.Get(embeddingsField).List()
	if embeddings.Len() == 0 {
		return "", fmt.Errorf("gateway embedding response did not include embeddings")
	}
	results := make([]map[string]any, 0, embeddings.Len())
	for i := 0; i < embeddings.Len(); i++ {
		result, err := e.registerEmbeddingResult(embeddings.Get(i).Message())
		if err != nil {
			return "", err
		}
		results = append(results, result)
	}

	var payload any = map[string]any{"embeddings": results}
	if len(results) == 1 {
		payload = results[0]
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode embedding result: %w", err)
	}
	return string(data), nil
}

func (e *Executor) registerEmbeddingResult(reflected protoreflect.Message) (map[string]any, error) {
	fields := reflected.Descriptor().Fields()
	vectorField := fields.ByName("vector")
	hashField := fields.ByName("content_hash")
	modelField := fields.ByName("model")
	dimensionsField := fields.ByName("dimensions")
	providerField := fields.ByName("provider")
	if vectorField == nil || hashField == nil || modelField == nil || dimensionsField == nil || providerField == nil {
		return nil, fmt.Errorf("gateway embedding item descriptor is missing expected fields")
	}

	list := reflected.Get(vectorField).List()
	vector := make([]float32, list.Len())
	for i := 0; i < list.Len(); i++ {
		vector[i] = float32(list.Get(i).Float())
	}
	contentHash := strings.TrimSpace(reflected.Get(hashField).String())
	if contentHash == "" {
		return nil, fmt.Errorf("gateway embedding response did not include contentHash")
	}

	e.mu.Lock()
	e.nextEmbedding++
	ref := fmt.Sprintf("emb_%d", e.nextEmbedding)
	now := time.Now()
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
	e.embeddingBorn[ref] = now
	e.embeddingBorn[contentHash] = now
	e.pending[ref] = struct{}{}
	e.mu.Unlock()

	return map[string]any{
		"embeddingRef": ref,
		"contentHash":  metadata["contentHash"],
		"dimensions":   metadata["dimensions"],
		"model":        metadata["model"],
		"provider":     metadata["provider"],
	}, nil
}

func (e *Executor) documentExtractTextToolResult(msg protoreflect.ProtoMessage, requestArguments string) (string, error) {
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
	sourceInfo := documentExtractionSourceInfo(requestArguments)
	sourceInfo["serviceFunction"] = "document_ExtractText"
	sourceInfo["sourceHash"] = sourceHash
	ref, info := e.registerContent(text, sourceInfo)
	payload["contentRef"] = mustJSONRaw(ref)
	payload["contentChars"] = mustJSONRaw(len([]rune(text)))
	payload["contentHash"] = mustJSONRaw(info["contentHash"])
	if sourceHash != "" {
		payload["sourceHash"] = mustJSONRaw(sourceHash)
	}
	registerPageReferences(e, payload, sourceInfo)
	compactDocumentExtractTextPayload(payload)
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode document text content reference: %w", err)
	}
	return string(out), nil
}

func registerPageReferences(e *Executor, payload map[string]json.RawMessage, sourceInfo map[string]any) {
	raw, ok := payload["pages"]
	if !ok {
		return
	}
	var pages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &pages); err != nil {
		return
	}
	for _, page := range pages {
		text := rawStringArgument(page, "text")
		if strings.TrimSpace(text) == "" {
			continue
		}
		info := cloneMetadata(sourceInfo)
		if rawPage, ok := page["pageNumber"]; ok {
			var pageNumber int
			if err := json.Unmarshal(rawPage, &pageNumber); err == nil {
				info["pageNumber"] = pageNumber
			}
		}
		info["modality"] = "text"
		ref, _ := e.registerContent(text, info)
		page["pageRef"] = mustJSONRaw(ref)
	}
	payload["pages"] = mustJSONRaw(pages)
}

func (e *Executor) ioReadToolResult(msg protoreflect.ProtoMessage, requestArguments string) (string, error) {
	reflected := msg.ProtoReflect()
	contentField := reflected.Descriptor().Fields().ByName("content")
	if contentField == nil {
		return "", fmt.Errorf("io read response descriptor is missing content field")
	}
	content := reflected.Get(contentField).String()

	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("encode io read response: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return string(data), nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("decode io read response for content reference: %w", err)
	}
	sourceInfo := ioReadSourceInfo(requestArguments)
	sourceInfo["serviceFunction"] = "io_Read"
	ref, info := e.registerContent(content, sourceInfo)
	payload["contentRef"] = mustJSONRaw(ref)
	payload["contentChars"] = mustJSONRaw(len([]rune(content)))
	payload["contentHash"] = mustJSONRaw(info["contentHash"])
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode io read content reference: %w", err)
	}
	return string(out), nil
}

func (e *Executor) ioReadMediaToolResult(msg protoreflect.ProtoMessage, _ string) (string, error) {
	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("encode io media response: %w", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("decode io media response: %w", err)
	}
	rawContent, ok := payload["content"]
	if !ok {
		return string(data), nil
	}
	var encoded string
	if err := json.Unmarshal(rawContent, &encoded); err != nil {
		return "", fmt.Errorf("decode io media content: %w", err)
	}
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode io media bytes: %w", err)
	}
	info := rawObjectMetadata(payload["source"])
	info["serviceFunction"] = "io_ReadMedia"
	ref, mediaInfo := e.registerMedia(content, info)
	delete(payload, "content")
	payload["mediaRef"] = mustJSONRaw(ref)
	payload["contentHash"] = mustJSONRaw(mediaInfo["contentHash"])
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode io media reference: %w", err)
	}
	return string(out), nil
}

func (e *Executor) documentMediaToolResult(msg protoreflect.ProtoMessage, _ string) (string, error) {
	data, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("encode document media response: %w", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("decode document media response: %w", err)
	}
	if raw, ok := payload["images"]; ok {
		payload["images"] = e.referenceImageArray(raw)
	}
	if raw, ok := payload["pages"]; ok {
		var pages []map[string]json.RawMessage
		if json.Unmarshal(raw, &pages) == nil {
			for _, page := range pages {
				if rawImages, ok := page["images"]; ok {
					page["images"] = e.referenceImageArray(rawImages)
				}
			}
			payload["pages"] = mustJSONRaw(pages)
		}
	}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode document media references: %w", err)
	}
	return string(out), nil
}

func (e *Executor) referenceImageArray(raw json.RawMessage) json.RawMessage {
	var images []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &images); err != nil {
		return raw
	}
	for _, image := range images {
		var encoded string
		if rawContent, ok := image["content"]; ok && json.Unmarshal(rawContent, &encoded) == nil {
			content, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil && len(content) > 0 {
				info := rawObjectMetadata(image["source"])
				info["serviceFunction"] = "document_ExtractImages"
				ref, metadata := e.registerMedia(content, info)
				image["mediaRef"] = mustJSONRaw(ref)
				image["contentHash"] = mustJSONRaw(metadata["contentHash"])
			}
			delete(image, "content")
		}
	}
	return mustJSONRaw(images)
}

func rawObjectMetadata(raw json.RawMessage) map[string]any {
	out := make(map[string]any)
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func ioReadSourceInfo(arguments string) map[string]any {
	info := make(map[string]any)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload); err != nil {
		return info
	}
	if rawPath, ok := payload["path"]; ok {
		var path string
		if err := json.Unmarshal(rawPath, &path); err == nil && strings.TrimSpace(path) != "" {
			info["path"] = strings.TrimSpace(path)
		}
	}
	return info
}

func documentExtractionSourceInfo(arguments string) map[string]any {
	info := make(map[string]any)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload); err != nil {
		return info
	}
	rawInput, ok := payload["input"]
	if !ok {
		return info
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(rawInput, &input); err != nil {
		return info
	}
	sourceURI := rawStringArgument(input, "sourceUri")
	if sourceURI == "" {
		sourceURI = rawStringArgument(input, "source_uri")
	}
	filename := rawStringArgument(input, "filename")
	mimeType := rawStringArgument(input, "mimeType")
	if mimeType == "" {
		mimeType = rawStringArgument(input, "mime_type")
	}
	if sourceURI != "" {
		info["sourceURI"] = sourceURI
	}
	if filename == "" {
		filename = filenameFromReference(sourceURI)
	}
	if filename != "" {
		info["filename"] = filename
	}
	if mimeType != "" {
		info["mimeType"] = mimeType
	}
	if rawMetadata, ok := input["metadata"]; ok {
		var metadata map[string]string
		if err := json.Unmarshal(rawMetadata, &metadata); err == nil {
			for key, value := range metadata {
				key = strings.TrimSpace(key)
				value = strings.TrimSpace(value)
				if key == "" || value == "" {
					continue
				}
				info[key] = value
			}
		}
	}
	return info
}

func (e *Executor) registerContent(content string, metadata map[string]any) (string, map[string]any) {
	sum := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(sum[:])
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextContent++
	ref := fmt.Sprintf("content_%d", e.nextContent)
	now := time.Now()
	info := cloneMetadata(metadata)
	info["contentHash"] = contentHash
	info["chars"] = len([]rune(content))
	e.contents[ref] = content
	e.contents[contentHash] = content
	e.contentInfo[ref] = cloneMetadata(info)
	e.contentInfo[contentHash] = cloneMetadata(info)
	e.contentBorn[ref] = now
	e.contentBorn[contentHash] = now
	return ref, cloneMetadata(info)
}

func (e *Executor) registerMedia(content []byte, metadata map[string]any) (string, map[string]any) {
	sum := sha256.Sum256(content)
	contentHash := hex.EncodeToString(sum[:])
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextMedia++
	ref := fmt.Sprintf("media_%d", e.nextMedia)
	now := time.Now()
	info := cloneMetadata(metadata)
	info["contentHash"] = contentHash
	info["bytes"] = len(content)
	e.media[ref] = append([]byte(nil), content...)
	e.media[contentHash] = append([]byte(nil), content...)
	e.mediaInfo[ref] = cloneMetadata(info)
	e.mediaInfo[contentHash] = cloneMetadata(info)
	e.mediaBorn[ref] = now
	e.mediaBorn[contentHash] = now
	return ref, cloneMetadata(info)
}

func (e *Executor) mediaByRef(ref string) ([]byte, map[string]any, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	content, ok := e.media[ref]
	if !ok {
		return nil, nil, false
	}
	return append([]byte(nil), content...), cloneMetadata(e.mediaInfo[ref]), true
}

func (e *Executor) contentByRef(ref string) (string, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	content, ok := e.contents[ref]
	return content, ok
}

func (e *Executor) contentMetadataByRef(ref string) (map[string]any, bool) {
	e.CleanupExpiredReferences(time.Now())
	e.mu.RLock()
	defer e.mu.RUnlock()
	metadata, ok := e.contentInfo[ref]
	return cloneMetadata(metadata), ok
}

func (e *Executor) attachResultReference(functionName, responseType string, data []byte) (string, error) {
	if len(data) < largeResultReferenceMin && !shouldReferenceResponse(responseType) {
		return string(data), nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return string(data), nil
	}
	ref, info := e.registerContent(string(data), map[string]any{
		"serviceFunction": functionName,
		"responseType":    responseType,
	})
	payload["resultRef"] = mustJSONRaw(ref)
	payload["resultChars"] = mustJSONRaw(info["chars"])
	payload["resultHash"] = mustJSONRaw(info["contentHash"])
	compactReferencedResultPayload(responseType, payload)
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode service result reference: %w", err)
	}
	return string(out), nil
}

func compactReferencedResultPayload(responseType string, payload map[string]json.RawMessage) {
	switch responseType {
	case "quark.indexer.v1.ContextResponse", "quark.indexer.v1.QueryContextResponse":
		compactContextResponsePayload(payload)
	}
}

func compactDocumentExtractTextPayload(payload map[string]json.RawMessage) {
	compactTextField(payload, "text", documentTextPreviewMax)
	compactDocumentPageTextArray(payload, "pages")
	payload["resultCompacted"] = mustJSONRaw(true)
}

func compactDocumentPageTextArray(payload map[string]json.RawMessage, key string) {
	raw, ok := payload[key]
	if !ok {
		return
	}
	var pages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &pages); err != nil {
		return
	}
	originalCount := len(pages)
	if originalCount > documentPagePreviewMax {
		pages = pages[:documentPagePreviewMax]
		payload[key+"Count"] = mustJSONRaw(originalCount)
		payload[key+"Truncated"] = mustJSONRaw(true)
	}
	for i := range pages {
		compactTextField(pages[i], "text", documentPageTextMax)
	}
	payload[key] = mustJSONRaw(pages)
}

func compactContextResponsePayload(payload map[string]json.RawMessage) {
	compactTextField(payload, "reasoningContext", reasoningPreviewMax)
	compactChunkArrayField(payload, "chunks")
	if raw, ok := payload["contextPackage"]; ok {
		var contextPackage map[string]json.RawMessage
		if err := json.Unmarshal(raw, &contextPackage); err == nil {
			compactChunkArrayField(contextPackage, "chunks")
			payload["contextPackage"] = mustJSONRaw(contextPackage)
		}
	}
	payload["resultCompacted"] = mustJSONRaw(true)
}

func compactChunkArrayField(payload map[string]json.RawMessage, key string) {
	raw, ok := payload[key]
	if !ok {
		return
	}
	var chunks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &chunks); err != nil {
		return
	}
	for i := range chunks {
		compactTextField(chunks[i], "text", contextTextPreviewMax)
	}
	payload[key] = mustJSONRaw(chunks)
}

func compactTextField(payload map[string]json.RawMessage, key string, maxRunes int) {
	if maxRunes <= 0 {
		return
	}
	raw, ok := payload[key]
	if !ok {
		return
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return
	}
	payload[key] = mustJSONRaw(string(runes[:maxRunes]) + "\n...[truncated; full content is available through the runtime reference in this tool result]")
	payload[key+"Chars"] = mustJSONRaw(len(runes))
	payload[key+"Truncated"] = mustJSONRaw(true)
}

func shouldReferenceResponse(responseType string) bool {
	switch responseType {
	case "quark.indexer.v1.ContextResponse",
		"quark.indexer.v1.QueryContextResponse",
		"quark.devops.v1.RunTestsResponse",
		"quark.devops.v1.ExplainFailureResponse",
		"quark.system.v1.ReadLogsResponse",
		"quark.system.v1.SnapshotResponse":
		return true
	default:
		return false
	}
}

func mustJSONRaw(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func (e *Executor) CleanupExpiredReferences(now time.Time) {
	if e == nil || e.refTTL <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for ref, born := range e.contentBorn {
		if now.Sub(born) <= e.refTTL {
			continue
		}
		delete(e.contentBorn, ref)
		delete(e.contents, ref)
		delete(e.contentInfo, ref)
	}
	for ref, born := range e.embeddingBorn {
		if now.Sub(born) <= e.refTTL {
			continue
		}
		delete(e.embeddingBorn, ref)
		delete(e.embeddings, ref)
		delete(e.embeddingInfo, ref)
		delete(e.pending, ref)
	}
	for ref, born := range e.mediaBorn {
		if now.Sub(born) <= e.refTTL {
			continue
		}
		delete(e.mediaBorn, ref)
		delete(e.media, ref)
		delete(e.mediaInfo, ref)
	}
}

func (e *Executor) setReferenceTTL(ttl time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.refTTL = ttl
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
