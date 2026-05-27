package workflow

import "github.com/quarkloop/pkg/plugin"

// ContextHistory removes completed mechanical exchanges only after they stop
// proving what the current step must build on. The initial run exchange remains
// visible through extraction and embedding so the model does not create a
// second run. Chunk writes remain visible while chunk persistence is pending
// so the model can distinguish completed records from remaining records
// without repeating successful writes.
func (t *Tracker) ContextHistory(messages []plugin.Message) []plugin.Message {
	if t == nil || len(messages) == 0 {
		return append([]plugin.Message(nil), messages...)
	}
	removable := make(map[string]struct{})
	for _, state := range t.states {
		if state.Kind != KindKnowledgeIndex {
			continue
		}
		step, ok := currentMissingStep(state)
		if !ok {
			for _, name := range completedKnowledgeWorkflowTools() {
				removable[name] = struct{}{}
			}
			continue
		}
		if step.ID == "index" || step.ID == "ingest-complete" {
			removable["runstate_StartRun"] = struct{}{}
		}
		if step.ID == "ingest-complete" {
			removable["indexer_UpsertChunk"] = struct{}{}
		}
	}
	if len(removable) == 0 {
		return append([]plugin.Message(nil), messages...)
	}
	return removeCompleteToolExchanges(messages, removable)
}

func completedKnowledgeWorkflowTools() []string {
	return []string{
		"runstate_StartRun",
		"document_ExtractText",
		"document_GetPages",
		"document_ExtractLayout",
		"document_ExtractTables",
		"document_ExtractImages",
		"document_RunOCR",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	}
}

func removeCompleteToolExchanges(messages []plugin.Message, removable map[string]struct{}) []plugin.Message {
	out := make([]plugin.Message, 0, len(messages))
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		if message.Role != "assistant" || len(message.ToolCalls) == 0 || !allToolCallsRemovable(message.ToolCalls, removable) {
			out = append(out, message)
			continue
		}
		callIDs := make(map[string]struct{}, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			callIDs[call.ID] = struct{}{}
		}
		for index+1 < len(messages) && messages[index+1].Role == "tool" {
			if _, ok := callIDs[messages[index+1].ToolCallID]; !ok {
				break
			}
			index++
		}
	}
	return out
}

func allToolCallsRemovable(calls []plugin.ToolCall, removable map[string]struct{}) bool {
	for _, call := range calls {
		if _, ok := removable[call.Function.Name]; !ok {
			return false
		}
	}
	return true
}
