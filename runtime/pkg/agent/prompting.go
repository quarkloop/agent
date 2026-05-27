package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/harnessclient"
	"github.com/quarkloop/runtime/pkg/llm"
	"github.com/quarkloop/runtime/pkg/workflow"
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

func (a *Agent) promptMaterialsForTools(tools []plugin.ToolSchema) []harnessclient.Material {
	materials := a.promptMaterials()
	filtered := make([]harnessclient.Material, 0, len(materials))
	for _, material := range materials {
		if material.SourceKind != "service_skill" || serviceMaterialHasExposedTool(material, tools) {
			filtered = append(filtered, material)
		}
	}
	return filtered
}

func serviceMaterialHasExposedTool(material harnessclient.Material, tools []plugin.ToolSchema) bool {
	if len(material.ApplicableTools) == 0 {
		return false
	}
	exposed := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if name := strings.TrimSpace(tool.Name); name != "" {
			exposed[name] = struct{}{}
		}
	}
	for _, name := range material.ApplicableTools {
		if _, ok := exposed[strings.TrimSpace(name)]; ok {
			return true
		}
	}
	return false
}

func (a *Agent) composeContext(ctx context.Context, history []plugin.Message, contextWindow int, workSummary string, materials, runtimeFacts []harnessclient.Material) ([]plugin.Message, error) {
	if a.config.ContextComposer == nil {
		return nil, fmt.Errorf("Harness context composer is required for agent inference")
	}
	facts := append([]harnessclient.Material(nil), runtimeFacts...)
	if workSummary != "" && workSummary != "No active work." {
		content, _ := json.Marshal(map[string]string{"type": "runtime.plan.status", "status": workSummary})
		facts = append(facts, harnessclient.Material{
			SourceID:   "runtime.plan.status",
			SourceKind: "runtime_fact",
			Content:    string(content),
		})
	}
	return a.config.ContextComposer.Compose(ctx, harnessclient.Input{
		Materials:     materials,
		RuntimeFacts:  facts,
		History:       append([]plugin.Message(nil), history...),
		ContextWindow: contextWindow,
	})
}

func (a *Agent) contextPreparer(contextWindow int, workSummary string) llm.ContextPreparer {
	return func(ctx context.Context, messages []plugin.Message) ([]plugin.Message, error) {
		return a.composeContext(ctx, messages, contextWindow, workSummary, a.promptMaterials(), nil)
	}
}

func (a *Agent) workflowContextPreparer(contextWindow int, workSummary string, tools []plugin.ToolSchema, surface llm.ToolSurface, tracker *workflow.Tracker) llm.ContextPreparer {
	return func(ctx context.Context, messages []plugin.Message) ([]plugin.Message, error) {
		currentTools := tools
		if surface != nil {
			currentTools = surface(append([]plugin.ToolSchema(nil), tools...))
		}
		history := messages
		if tracker != nil {
			history = tracker.ContextHistory(messages)
		}
		var facts []harnessclient.Material
		if tracker != nil {
			if status := strings.TrimSpace(tracker.ContinuationStatus()); status != "" {
				facts = append(facts, harnessclient.Material{
					SourceID:   "runtime.workflow.status",
					SourceKind: "runtime_fact",
					Content:    status,
					Required:   true,
				})
			}
		}
		return a.composeContext(ctx, history, contextWindow, workSummary, a.promptMaterialsForTools(currentTools), facts)
	}
}
