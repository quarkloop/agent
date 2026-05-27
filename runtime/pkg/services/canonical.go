package services

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// NormalizeToolCallArguments intentionally does not author canonical index
// records. Semantic fields belong to the coordinating agent. For page
// references, the runtime restores provenance established when it issued the
// opaque reference so workflow validation can verify the same evidence.
func (e *Executor) NormalizeToolCallArguments(_ context.Context, name, arguments string) (string, error) {
	if e != nil && strings.TrimSpace(name) == "gateway_Embed" {
		return e.normalizeGatewayEmbeddingReferenceProvenance(arguments)
	}
	return arguments, nil
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
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
