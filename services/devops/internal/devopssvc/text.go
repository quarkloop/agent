package devopssvc

import (
	"strings"
)

func failureLines(logs string) []string {
	out := make([]string, 0)
	for _, line := range nonEmptyLines(logs) {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "fail") || strings.Contains(lower, "error") || strings.Contains(lower, "panic") {
			out = append(out, line)
		}
	}
	if len(out) > 20 {
		return out[:20]
	}
	return out
}

func nonEmptyLines(value string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
