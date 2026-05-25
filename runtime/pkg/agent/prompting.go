package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/guard"
	"github.com/quarkloop/runtime/pkg/harnessclient"
	"github.com/quarkloop/runtime/pkg/llm"
)

func (a *Agent) promptMaterials() []harnessclient.Material {
	materials := make([]harnessclient.Material, 0, len(a.config.PromptMaterials)+1)
	if a.profile.SystemPrompt != "" {
		materials = append(materials, harnessclient.Material{
			SourceID:   "plugin.agent." + a.profile.ID + ".system",
			SourceKind: "agent_system",
			Content:    a.profile.SystemPrompt,
			Required:   true,
		})
	}
	materials = append(materials, a.config.PromptMaterials...)
	return materials
}

func (a *Agent) composeContext(ctx context.Context, history []plugin.Message, contextWindow int, workSummary string) ([]plugin.Message, error) {
	if a.config.ContextComposer == nil {
		return nil, fmt.Errorf("Harness context composer is required for agent inference")
	}
	var facts []harnessclient.Material
	if workSummary != "" && workSummary != "No active work." {
		content, _ := json.Marshal(map[string]string{"type": "runtime.plan.status", "status": workSummary})
		facts = append(facts, harnessclient.Material{
			SourceID:   "runtime.plan.status",
			SourceKind: "runtime_fact",
			Content:    string(content),
		})
	}
	return a.config.ContextComposer.Compose(ctx, harnessclient.Input{
		Materials:     a.promptMaterials(),
		RuntimeFacts:  facts,
		History:       append([]plugin.Message(nil), history...),
		ContextWindow: contextWindow,
	})
}

func (a *Agent) contextPreparer(contextWindow int, workSummary string) llm.ContextPreparer {
	return func(ctx context.Context, messages []plugin.Message) ([]plugin.Message, error) {
		return a.composeContext(ctx, messages, contextWindow, workSummary)
	}
}

func (a *Agent) finalGuard() llm.FinalGuard {
	return guard.PendingEmbeddingRefs(a.config.PendingRefs, 8)
}
