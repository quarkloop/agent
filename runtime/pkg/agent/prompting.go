package agent

import (
	"github.com/quarkloop/runtime/pkg/guard"
	"github.com/quarkloop/runtime/pkg/llm"
	"github.com/quarkloop/runtime/pkg/prompt"
)

func (a *Agent) systemPrompt() string {
	addenda := make([]string, 0, len(a.config.PromptAddenda)+1)
	addenda = append(addenda, a.config.PromptAddenda...)
	if block := a.handoff.PromptBlock(); block != "" {
		addenda = append(addenda, block)
	}
	return prompt.BuildRuntimeSystemPrompt(a.profile.SystemPrompt, addenda)
}

func (a *Agent) finalGuard() llm.FinalGuard {
	return guard.PendingEmbeddingRefs(a.config.PendingRefs, 8)
}
