package workflow

import (
	"fmt"
	"strings"
)

func (t *Tracker) PromptBlock() string {
	if t == nil || len(t.states) == 0 {
		return ""
	}
	sections := make([]string, 0, len(t.states)+1)
	sections = append(sections, "Active Runtime Workflow\nThe runtime detected one or more service-backed workflows for this user request. Complete the listed service-backed steps before finalizing. Use natural reasoning for the user-facing answer, but use the available service functions for durable work.")
	for _, state := range t.states {
		sections = append(sections, workflowPromptBlock(state))
	}
	return strings.Join(sections, "\n\n")
}

// ContinuationPrompt returns the enforcement instruction for the next model
// turn after a service result; workflow policy owns this instruction until
// prompt assembly moves behind the dedicated harness boundary.
func (t *Tracker) ContinuationPrompt() string {
	if t == nil || len(t.states) == 0 {
		return ""
	}
	if len(t.missing()) == 0 {
		return "Runtime workflow completion: the required service-backed workflow steps are complete. Produce the final user-facing answer now from the existing service evidence. Do not call more tools unless a required service call failed."
	}
	parts := make([]string, 0, len(t.states))
	for _, state := range t.states {
		if stateComplete(state) {
			continue
		}
		step, ok := currentMissingStep(state)
		if !ok {
			continue
		}
		parts = append(parts, continuationPromptForStep(state, step))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Runtime workflow continuation: respond with tool calls only until the required workflow steps are complete; do not draft a user-facing final answer yet.\n\n" + strings.Join(parts, "\n\n")
}

func workflowPromptBlock(state State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s workflow requirements:\n", state.Kind)
	for i, step := range state.Steps {
		fmt.Fprintf(&b, "%d. %s using one of: %s.\n", i+1, step.Label, strings.Join(step.AnyOf, ", "))
	}
	switch state.Kind {
	case KindKnowledgeIndex:
		b.WriteString("For document indexing, start a durable run with one document item per discovered source before per-source work, extract every source through document service functions, produce embeddings for canonical chunks, persist each canonical chunk with indexer_UpsertChunk using embeddingRef values returned by gateway_Embed, then close the durable run with runstate_MarkComplete. Every indexer_UpsertChunk call for source documents must be a complete canonical knowledge record with document, sourceMetadata, provenance, non-empty facts, non-empty entities, relations, citations, and either textContentRef or textContent. For batch work, issue all known calls for the same step in one ordered tool-call batch when possible. Never provide manual embedding arrays to indexer functions. Do not use document-only or legacy indexer calls as a substitute for canonical chunk indexing. runstate_UpdateItemState may record per-item progress, but it is not the terminal batch completion step. Do not final-answer that indexing is done until runstate_MarkComplete succeeds after the source content is indexed. After runstate_MarkComplete succeeds, answer immediately and concisely; do not call more tools unless the terminal call failed.")
	case KindKnowledgeQuery:
		b.WriteString("For knowledge questions, retrieve from the index with a queryVectorRef returned by gateway_Embed before answering and use citation or grounding functions when they are available for the workflow. Never provide manual query vectors to indexer functions. Do not reread original files unless retrieval is insufficient and the user asks for repair or reindexing. After citation or grounding succeeds, answer immediately and concisely from the retrieved evidence; do not call more tools unless retrieval or grounding failed.")
	case KindDevOps:
		b.WriteString("For DevOps work, inspect repository state before running build, test, release, or policy actions. Report only evidence from service results and clearly distinguish dry-run planning from mutations.")
	case KindSystemInspect, KindSystemMutation:
		b.WriteString("For system work, gather read-only inspection evidence before proposing actions. Mutation functions require the configured approval and policy path.")
	}
	return strings.TrimSpace(b.String())
}

func continuationPromptForStep(state State, step Step) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s current required step: %s", state.Kind, step.Label)
	if progress := stepProgress(step); progress != "" {
		fmt.Fprintf(&b, " (%s)", progress)
	}
	fmt.Fprintf(&b, ". Use one of: %s.", strings.Join(step.AnyOf, ", "))
	if detail := continuationDetail(state, step.ID); detail != "" {
		b.WriteString(" ")
		b.WriteString(detail)
	}
	return b.String()
}

func stepProgress(step Step) string {
	required := requiredCount(step)
	if required <= 1 {
		return "not complete"
	}
	remaining := required - step.CompletedCount
	if remaining < 0 {
		remaining = 0
	}
	return fmt.Sprintf("%d of %d complete; %d remaining", step.CompletedCount, required, remaining)
}

func continuationDetail(state State, stepID string) string {
	switch state.Kind {
	case KindKnowledgeIndex:
		return knowledgeIndexContinuationDetail(stepID, state.RunID)
	case KindKnowledgeQuery:
		return knowledgeQueryContinuationDetail(stepID)
	case KindDevOps:
		return "Continue from the existing service evidence and use the next DevOps service function before summarizing."
	case KindSystemInspect, KindSystemMutation:
		return "Use read-only inspection evidence first; mutation functions must follow the configured approval and policy path."
	default:
		return ""
	}
}

func knowledgeIndexContinuationDetail(stepID, runID string) string {
	switch stepID {
	case "ingest-start":
		return "If the document list is not known, inspect the user-provided location with io_List or io_Stat, then call runstate_StartRun once with one item for every discovered source."
	case "extract":
		return "Extract every remaining source with text/page extraction so source text is available for semantic structuring, embedding, and indexing. Metadata-only parsing does not satisfy this step. Do not use io_Read or io_ExtractPdf as a substitute for extraction and do not embed or index before extraction is complete."
	case "embed":
		return "Create embeddings for the remaining extracted canonical chunks. Batch all known remaining embedding calls in one assistant turn when possible."
	case "index":
		detail := "Persist each remaining canonical chunk with indexer_UpsertChunk using embeddingRef values returned by gateway_Embed. Each upsert must include document, sourceMetadata, provenance, facts, entities, relations, citations, and either textContentRef or textContent. After the final remaining upsert succeeds, close the durable run with runstate_MarkComplete before answering."
		if runID != "" {
			detail += fmt.Sprintf(" Use runId %q when the workflow reaches runstate_MarkComplete.", runID)
		}
		return detail
	case "ingest-complete":
		if runID != "" {
			return fmt.Sprintf("Call runstate_MarkComplete now with runId %q. After it succeeds, answer briefly with what was indexed.", runID)
		}
		return "Call runstate_MarkComplete now with the runId returned by runstate_StartRun. After it succeeds, answer briefly with what was indexed."
	default:
		return ""
	}
}

func knowledgeQueryContinuationDetail(stepID string) string {
	switch stepID {
	case "embed-query":
		return "Embed the user question first. Do not invent query vectors."
	case "retrieve":
		return "Retrieve from the index using the query vector reference returned by gateway_Embed."
	case "ground":
		return "Verify or render citations from retrieved evidence before answering."
	default:
		return ""
	}
}
