package devopssvc

import (
	"strings"
)

const (
	maxEvidenceLines     = 20
	maxEvidenceLineRunes = 512
)

func boundedEvidence(values []string) []string {
	out := make([]string, 0, min(len(values), maxEvidenceLines))
	for _, line := range values {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > maxEvidenceLineRunes {
			line = string(runes[:maxEvidenceLineRunes]) + "...[truncated]"
		}
		out = append(out, line)
		if len(out) == maxEvidenceLines {
			break
		}
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
