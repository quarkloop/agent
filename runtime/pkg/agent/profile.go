package agent

import "strings"

// Profile is the runtime-owned projection of a supervisor-resolved agent
// profile. It intentionally contains only fields the agent loop needs.
type Profile struct {
	ID             string
	Name           string
	Description    string
	SystemPrompt   string
	HandoffTargets []string
}

func (p Profile) normalize(fallbackID, fallbackName, fallbackDescription, fallbackPrompt string) Profile {
	out := Profile{
		ID:           firstNonEmpty(p.ID, fallbackID),
		Name:         firstNonEmpty(p.Name, fallbackName),
		Description:  firstNonEmpty(p.Description, fallbackDescription),
		SystemPrompt: firstNonEmpty(p.SystemPrompt, fallbackPrompt),
	}
	out.HandoffTargets = copyStrings(p.HandoffTargets)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
