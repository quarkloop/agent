package workflow

import (
	"encoding/json"
	"strings"
)

func observedItemCount(result string) int {
	var payload any
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &payload); err != nil {
		return 0
	}
	return maxItemArrayLen(payload)
}

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

func maxItemArrayLen(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		maxCount := 0
		for key, child := range typed {
			count := maxItemArrayLen(child)
			if strings.EqualFold(key, "items") {
				if items, ok := child.([]any); ok && len(items) > count {
					count = len(items)
				}
			}
			if count > maxCount {
				maxCount = count
			}
		}
		return maxCount
	case []any:
		maxCount := 0
		for _, child := range typed {
			if count := maxItemArrayLen(child); count > maxCount {
				maxCount = count
			}
		}
		return maxCount
	default:
		return 0
	}
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
