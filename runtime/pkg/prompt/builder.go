package prompt

import (
	"strings"

	"github.com/quarkloop/runtime/pkg/extraction"
	"github.com/quarkloop/runtime/pkg/workspace"
)

// SystemInput contains the resolved inputs needed to build the runtime system
// prompt for an agent session.
type SystemInput struct {
	BasePrompt    string
	RuntimeBlocks []string
	Addenda       []string
}

// BuildSystemPrompt builds a normalized system prompt from resolved profile
// content, runtime-owned guidance blocks, and supervisor-provided addenda.
func BuildSystemPrompt(input SystemInput) string {
	sections := make([]string, 0, 1+len(input.RuntimeBlocks)+len(input.Addenda))
	basePrompt := strings.TrimSpace(input.BasePrompt)
	if basePrompt == "" {
		basePrompt = strings.TrimSpace(GetSystemPrompt())
	}
	if basePrompt != "" {
		sections = append(sections, basePrompt)
	}
	for _, block := range input.RuntimeBlocks {
		if block = strings.TrimSpace(block); block != "" {
			sections = append(sections, block)
		}
	}
	for _, addendum := range input.Addenda {
		if addendum = strings.TrimSpace(addendum); addendum != "" {
			sections = append(sections, addendum)
		}
	}
	return strings.Join(sections, "\n\n")
}

// DefaultRuntimeBlocks returns prompt blocks owned by runtime subsystems.
func DefaultRuntimeBlocks() []string {
	return []string{
		extraction.DefaultRegistry().PromptBlock(),
		workspace.PromptBlock(),
	}
}

// BuildRuntimeSystemPrompt builds the standard agent system prompt from a
// resolved profile prompt and runtime defaults.
func BuildRuntimeSystemPrompt(basePrompt string, addenda []string) string {
	return BuildSystemPrompt(SystemInput{
		BasePrompt:    basePrompt,
		RuntimeBlocks: DefaultRuntimeBlocks(),
		Addenda:       addenda,
	})
}
