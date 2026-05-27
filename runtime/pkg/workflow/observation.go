package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/quarkloop/runtime/pkg/sourceid"
)

func observedRunStateRunID(result string) string {
	var payload any
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &payload); err != nil {
		return ""
	}
	return findRunStateRunID(payload)
}

func findRunStateRunID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if run, ok := typed["run"]; ok {
			if id := runObjectID(run); id != "" {
				return id
			}
		}
		for _, child := range typed {
			if id := findRunStateRunID(child); id != "" {
				return id
			}
		}
	case []any:
		for _, child := range typed {
			if id := findRunStateRunID(child); id != "" {
				return id
			}
		}
	}
	return ""
}

func runObjectID(value any) string {
	run, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if id, ok := run["id"].(string); ok {
		return strings.TrimSpace(id)
	}
	return ""
}

func toolResultSucceeded(result string, err error) bool {
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(result)
	if trimmed == "" || strings.HasPrefix(trimmed, "error:") {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if isError, ok := payload["is_error"].(bool); ok && isError {
			return false
		}
		if success, ok := payload["success"].(bool); ok {
			return success
		}
	}
	return true
}

func workflowCompletionIdentities(stepID, name, arguments, result string) []string {
	if stepID == "embed" && name == "gateway_Embed" {
		identities := embeddingInputIdentities(arguments)
		count := successfulEmbeddingCount(result)
		if len(identities) > 0 {
			if count < len(identities) {
				identities = identities[:count]
			}
			return identities
		}
	}
	return []string{workflowCompletionIdentity(stepID, name, arguments)}
}

func successfulEmbeddingCount(result string) int {
	var payload map[string]any
	if json.Unmarshal([]byte(result), &payload) != nil {
		return 1
	}
	if embeddings, ok := payload["embeddings"].([]any); ok && len(embeddings) > 0 {
		return len(embeddings)
	}
	return 1
}

func embeddingInputIdentities(arguments string) []string {
	var payload struct {
		Inputs []struct {
			Content []struct {
				Ref  string `json:"ref"`
				Text string `json:"text"`
			} `json:"content"`
			Metadata map[string]any `json:"metadata"`
		} `json:"inputs"`
	}
	if json.Unmarshal([]byte(arguments), &payload) != nil || len(payload.Inputs) == 0 {
		return nil
	}
	identities := make([]string, 0, len(payload.Inputs))
	for _, input := range payload.Inputs {
		if source := firstStringByKeys(input.Metadata, "sourceUri", "source_uri"); source != "" {
			identities = append(identities, "source:"+sourceid.Canonical(source))
			continue
		}
		var text strings.Builder
		ref := ""
		for _, content := range input.Content {
			if ref == "" && strings.TrimSpace(content.Ref) != "" {
				ref = strings.TrimSpace(content.Ref)
			}
			text.WriteString(content.Text)
		}
		switch {
		case ref != "":
			identities = append(identities, "ref:"+ref)
		case text.Len() > 0:
			identities = append(identities, textIdentity(text.String()))
		default:
			return nil
		}
	}
	return identities
}

func workflowCompletionIdentity(stepID, name, arguments string) string {
	var payload any
	if json.Unmarshal([]byte(arguments), &payload) != nil {
		return ""
	}
	switch {
	case stepID == "extract" && (name == "document_ExtractText" || name == "document_GetPages"):
		return sourceid.Canonical(firstStringByKeys(payload, "sourceUri", "source_uri", "path"))
	case stepID == "embed" && name == "gateway_Embed":
		if ref := firstStringByKeys(payload, "contentRef", "pageRef", "mediaRef", "ref"); ref != "" {
			return "ref:" + ref
		}
		if text := firstStringByKeys(payload, "text"); text != "" {
			return textIdentity(text)
		}
	case stepID == "index" && name == "indexer_UpsertChunk":
		if source := firstStringByKeys(payload, "sourceUri", "source_uri"); source != "" {
			return "source:" + sourceid.Canonical(source)
		}
		if ref := firstStringByKeys(payload, "textContentRef"); ref != "" {
			return "ref:" + ref
		}
		if id := firstStringByKeys(payload, "chunkId"); id != "" {
			return "chunk:" + id
		}
	}
	return ""
}

func textIdentity(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "text:" + hex.EncodeToString(sum[:])
}

func firstStringByKeys(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
		for _, nested := range typed {
			if text := firstStringByKeys(nested, keys...); text != "" {
				return text
			}
		}
	case []any:
		for _, nested := range typed {
			if text := firstStringByKeys(nested, keys...); text != "" {
				return text
			}
		}
	}
	return ""
}
