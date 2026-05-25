package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/plugin"
)

func TestDetectKnowledgeIndexWorkflowFromUserLanguage(t *testing.T) {
	intents := Detect("Please index these PDF files so I can ask questions later.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	))
	if len(intents) != 1 || intents[0].Kind != KindKnowledgeIndex {
		t.Fatalf("intents = %+v", intents)
	}
	if len(intents[0].Steps) != 5 {
		t.Fatalf("steps = %+v", intents[0].Steps)
	}
}

func TestDetectKnowledgeIndexWorkflowTracksExplicitBatchCount(t *testing.T) {
	intents := Detect("Please index all 4 Markdown documents in this directory.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	))
	if len(intents) != 1 {
		t.Fatalf("intents = %+v", intents)
	}
	steps := stepsByID(intents[0].Steps)
	if got := steps["extract"].RequiredCount; got != 4 {
		t.Fatalf("extract required count = %d, want 4", got)
	}
	if got := steps["embed"].RequiredCount; got != 4 {
		t.Fatalf("embed required count = %d, want 4", got)
	}
	if got := steps["index"].RequiredCount; got != 4 {
		t.Fatalf("index required count = %d, want 4", got)
	}
}

func TestKnowledgeIndexWorkflowDoesNotTreatMetadataParsingAsContentExtraction(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 1 PDF file.", toolSchemas(
		"runstate_StartRun",
		"document_ParseBytes",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	if _, err := wrapped(context.Background(), "runstate_StartRun", `{"run":{"items":[{"id":"s1"}]}}`); err != nil {
		t.Fatalf("start run: %v", err)
	}

	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "document_ParseBytes"},
	}})
	if !retry || !strings.Contains(instruction, "document content extraction") {
		t.Fatalf("metadata-only parse should be blocked as extraction: %q retry=%t", instruction, retry)
	}
	if _, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "document_ExtractText"},
	}}); retry {
		t.Fatal("text extraction should satisfy the current extraction step")
	}

	prompt := tracker.ContinuationPrompt()
	if strings.Contains(prompt, "document_ParseBytes") {
		t.Fatalf("content extraction continuation should not advertise metadata-only parsing:\n%s", prompt)
	}
}

func TestDetectKnowledgeQueryDoesNotTreatCatalogAsIndexingVerb(t *testing.T) {
	intents := Detect("Search the indexed IT company documents and tell me which catalog item has SKU QOP-OBS-START.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_QueryContext",
		"citation_RenderReferences",
	))
	if len(intents) != 1 || intents[0].Kind != KindKnowledgeQuery {
		t.Fatalf("intents = %+v", intents)
	}
}

func TestDetectSystemAndDevOpsWorkflows(t *testing.T) {
	system := Detect("What is using disk and ports on this machine?", toolSchemas(
		"system_GetDiskUsage",
		"system_ListPorts",
	))
	if len(system) != 1 || system[0].Kind != KindSystemInspect || len(system[0].Steps) != 2 {
		t.Fatalf("system intents = %+v", system)
	}

	devops := Detect("Run the tests and explain any failure in this repository.", toolSchemas(
		"repo_Status",
		"test_RunTests",
		"test_ExplainFailure",
	))
	if len(devops) != 1 || devops[0].Kind != KindDevOps || len(devops[0].Steps) != 3 {
		t.Fatalf("devops intents = %+v", devops)
	}
}

func TestDetectDevOpsReleaseWorkflowRequiresPolicyAndProjectContext(t *testing.T) {
	devops := Detect("Inspect this repository for the Go project and preview the release plan without publishing.", toolSchemas(
		"repo_Status",
		"build_DetectProject",
		"build_DryRunRelease",
		"policy_EvaluateChange",
	))
	if len(devops) != 1 || devops[0].Kind != KindDevOps {
		t.Fatalf("devops intents = %+v", devops)
	}
	if len(devops[0].Steps) != 4 {
		t.Fatalf("devops steps = %+v", devops[0].Steps)
	}
}

func TestDetectDevOpsReleasePreviewDoesNotRequireGenericBuildExecution(t *testing.T) {
	devops := Detect("Use release automation to inspect the repository and preview the release plan using build_release.json without publishing.", toolSchemas(
		"repo_Status",
		"build_DetectProject",
		"build_RunTask",
		"build_DryRunRelease",
		"policy_EvaluateChange",
	))
	if len(devops) != 1 || devops[0].Kind != KindDevOps {
		t.Fatalf("devops intents = %+v", devops)
	}
	for _, step := range devops[0].Steps {
		if step.ID == "build" {
			t.Fatalf("release preview should not require generic build execution: %+v", devops[0].Steps)
		}
	}
}

func TestTrackerBlocksFinalizationUntilRequiredServiceResults(t *testing.T) {
	store := NewStore()
	var events []Event
	tracker := NewTracker("session-1", "Index these documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), store, func(event Event) {
		events = append(events, event)
	})
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	guard, retry := tracker.FinalGuard("")
	if !retry || !strings.Contains(guard, "durable run creation") {
		t.Fatalf("guard = %q retry=%t", guard, retry)
	}

	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"ok": true}`, nil
	})
	for _, tool := range []string{"runstate_StartRun", "document_ExtractText", "gateway_Embed", "indexer_UpsertChunk", "runstate_MarkComplete"} {
		if _, err := wrapped(context.Background(), tool, "{}"); err != nil {
			t.Fatalf("wrapped tool %s: %v", tool, err)
		}
	}
	if guard, retry := tracker.FinalGuard(""); retry || guard != "" {
		t.Fatalf("guard after completion = %q retry=%t", guard, retry)
	}
	if len(store.List("session-1")) != 1 || store.List("session-1")[0].Status != "complete" {
		t.Fatalf("stored workflow state = %+v", store.List("session-1"))
	}
	if len(events) == 0 {
		t.Fatal("expected workflow events")
	}
}

func TestTrackerAcceptsFinalBeforeRedundantCompletedStepToolCalls(t *testing.T) {
	store := NewStore()
	tracker := NewTracker("session-1", "Index these documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), store, nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"ok": true}`, nil
	})
	for _, tool := range []string{"runstate_StartRun", "document_ExtractText", "gateway_Embed", "indexer_UpsertChunk", "runstate_MarkComplete"} {
		if _, err := wrapped(context.Background(), tool, "{}"); err != nil {
			t.Fatalf("wrapped tool %s: %v", tool, err)
		}
	}

	if !tracker.AcceptFinalBeforeToolCalls("Indexed all documents.", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "runstate_MarkComplete"},
	}}) {
		t.Fatal("expected completed workflow to accept final content before redundant tool call")
	}
	if tracker.AcceptFinalBeforeToolCalls("Indexed all documents.", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "citation_RenderReferences"},
	}}) {
		t.Fatal("unexpected acceptance for a tool that is not a completed workflow step")
	}
}

func TestTrackerAcceptsFinalAfterTerminalStepToolCallCompletes(t *testing.T) {
	store := NewStore()
	tracker := NewTracker("session-1", "Index these documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), store, nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, tool := range []string{"runstate_StartRun", "document_ExtractText", "gateway_Embed", "indexer_UpsertChunk"} {
		if _, err := wrapped(context.Background(), tool, "{}"); err != nil {
			t.Fatalf("wrapped tool %s: %v", tool, err)
		}
	}
	terminal := []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "runstate_MarkComplete"},
	}}
	if tracker.AcceptFinalAfterToolCalls("Indexed all documents.", terminal) {
		t.Fatal("terminal final content was accepted before terminal tool success")
	}
	if _, err := wrapped(context.Background(), "runstate_MarkComplete", "{}"); err != nil {
		t.Fatalf("wrapped terminal tool: %v", err)
	}
	if !tracker.AcceptFinalAfterToolCalls("Indexed all documents.", terminal) {
		t.Fatal("expected final content to be accepted after terminal workflow tool success")
	}
}

func TestTrackerPromptBlockNamesRequiredKnowledgeIndexOrder(t *testing.T) {
	tracker := NewTracker("session-1", "Please index these Markdown files.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_UpdateItemState",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	block := tracker.PromptBlock()
	for _, want := range []string{
		"Active Runtime Workflow",
		"durable run creation",
		"document content extraction",
		"canonical indexing",
		"runstate_MarkComplete",
		"not the terminal batch completion step",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("prompt block missing %q:\n%s", want, block)
		}
	}
}

func TestTrackerGuardsFinalContentWithNonAdvancingToolCalls(t *testing.T) {
	var events []Event
	tracker := NewTracker("session-1", "Index these documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_UpdateItemState",
		"runstate_MarkComplete",
	), NewStore(), func(event Event) {
		events = append(events, event)
	})
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"ok": true}`, nil
	})
	for _, tool := range []string{"runstate_StartRun", "document_ExtractText", "gateway_Embed", "indexer_UpsertChunk"} {
		if _, err := wrapped(context.Background(), tool, "{}"); err != nil {
			t.Fatalf("wrapped tool %s: %v", tool, err)
		}
	}

	instruction, retry := tracker.GuardToolCalls("The batch is indexed.", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "runstate_UpdateItemState"},
	}})
	if !retry || !strings.Contains(instruction, "runstate_MarkComplete") {
		t.Fatalf("guard instruction = %q retry=%t", instruction, retry)
	}
	if _, retry := tracker.GuardToolCalls("The batch is indexed.", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "runstate_MarkComplete"},
	}}); retry {
		t.Fatal("guard should allow a tool call that completes a missing workflow step")
	}
	if len(events) == 0 || events[len(events)-1].Type != "blocked_tool_calls" {
		t.Fatalf("events = %+v", events)
	}
}

func TestTrackerIgnoresFailedServiceResults(t *testing.T) {
	store := NewStore()
	tracker := NewTracker("session-1", "Find information in the indexed PDFs.", toolSchemas(
		"gateway_Embed",
		"indexer_QueryContext",
	), store, nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": false}`, nil
	})
	if _, err := wrapped(context.Background(), "gateway_Embed", "{}"); err != nil {
		t.Fatalf("wrapped tool: %v", err)
	}
	if guard, retry := tracker.FinalGuard(""); !retry || !strings.Contains(guard, "query embedding") {
		t.Fatalf("failed result should not complete workflow: %q retry=%t", guard, retry)
	}
}

func TestTrackerUsesRunStateItemsForBatchCounts(t *testing.T) {
	tracker := NewTracker("session-1", "Please index these documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(_ context.Context, name, _ string) (string, error) {
		if name == "runstate_StartRun" {
			return `{"run":{"items":[{"id":"s1"},{"id":"s2"},{"id":"s3"}]}}`, nil
		}
		return `{"success": true}`, nil
	})
	if _, err := wrapped(context.Background(), "runstate_StartRun", "{}"); err != nil {
		t.Fatalf("start run: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := wrapped(context.Background(), "document_ExtractText", "{}"); err != nil {
			t.Fatalf("extract %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := wrapped(context.Background(), "gateway_Embed", "{}"); err != nil {
			t.Fatalf("embed %d: %v", i, err)
		}
	}
	guard, retry := tracker.FinalGuard("")
	if !retry || !strings.Contains(guard, "embedding generation (2 of 3 complete; 1 remaining)") {
		t.Fatalf("guard = %q retry=%t", guard, retry)
	}
}

func TestTrackerCarriesRunStateRunIDIntoTerminalContinuation(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(_ context.Context, name, _ string) (string, error) {
		if name == "runstate_StartRun" {
			return `{"run":{"id":"run-123","items":[{"id":"s1"},{"id":"s2"}]}}`, nil
		}
		return `{"success": true}`, nil
	})
	for _, tool := range []string{
		"runstate_StartRun",
		"document_ExtractText",
		"document_ExtractText",
		"gateway_Embed",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"indexer_UpsertChunk",
	} {
		if _, err := wrapped(context.Background(), tool, "{}"); err != nil {
			t.Fatalf("wrapped tool %s: %v", tool, err)
		}
	}

	prompt := tracker.ContinuationPrompt()
	for _, want := range []string{"durable run completion", "runstate_MarkComplete", `runId "run-123"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestTrackerBlocksExtraWorkflowCallsAfterCompletion(t *testing.T) {
	tracker := NewTracker("session-1", "Search the indexed PDFs and answer from the documents.", toolSchemas(
		"gateway_Embed",
		"indexer_QueryContext",
		"citation_VerifyGrounding",
		"citation_ResolveSpans",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, tool := range []string{"gateway_Embed", "indexer_QueryContext", "citation_VerifyGrounding"} {
		if _, err := wrapped(context.Background(), tool, "{}"); err != nil {
			t.Fatalf("wrapped tool %s: %v", tool, err)
		}
	}

	if prompt := tracker.ContinuationPrompt(); !strings.Contains(prompt, "workflow steps are complete") || !strings.Contains(prompt, "Produce the final user-facing answer") {
		t.Fatalf("completion prompt did not tell the model to answer:\n%s", prompt)
	}
	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "citation_ResolveSpans"},
	}})
	if !retry || !strings.Contains(instruction, "workflow is already complete") {
		t.Fatalf("extra workflow call was not blocked: %q retry=%t", instruction, retry)
	}
	if instruction, retry := tracker.GuardToolCalls("The answer is ready.", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "citation_VerifyGrounding"},
	}}); retry {
		t.Fatalf("redundant completed-step call with final content should remain compatible: %q", instruction)
	}
}

func TestTrackerContinuationPromptNamesOnlyCurrentStep(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 3 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(_ context.Context, name, _ string) (string, error) {
		if name == "runstate_StartRun" {
			return `{"run":{"items":[{"id":"s1"},{"id":"s2"},{"id":"s3"}]}}`, nil
		}
		return `{"success": true}`, nil
	})
	for _, tool := range []string{"runstate_StartRun", "document_ExtractText", "document_ExtractText", "document_ExtractText", "gateway_Embed"} {
		if _, err := wrapped(context.Background(), tool, "{}"); err != nil {
			t.Fatalf("wrapped tool %s: %v", tool, err)
		}
	}

	prompt := tracker.ContinuationPrompt()
	for _, want := range []string{
		"respond with tool calls only",
		"embedding generation",
		"1 of 3 complete; 2 remaining",
		"gateway_Embed",
		"Batch all known remaining embedding calls",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, notWant := range []string{"canonical indexing", "indexer_UpsertChunk"} {
		if strings.Contains(prompt, notWant) {
			t.Fatalf("continuation prompt should not name future step %q:\n%s", notWant, prompt)
		}
	}
}

func TestTrackerFinalGuardUsesCurrentContinuationInstruction(t *testing.T) {
	tracker := NewTracker("session-1", "Please index these documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	guard, retry := tracker.FinalGuard("done")
	if !retry {
		t.Fatal("expected final guard retry")
	}
	if !strings.Contains(guard, "current required step: durable run creation") {
		t.Fatalf("guard did not focus on current step:\n%s", guard)
	}
	if strings.Contains(guard, "canonical indexing") {
		t.Fatalf("guard should not emphasize future steps:\n%s", guard)
	}
}

func TestTrackerBlocksNonAdvancingCallsWhenBatchIndexIsCurrentStep(t *testing.T) {
	var events []Event
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertDocument",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), func(event Event) {
		events = append(events, event)
	})
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, tool := range []string{"runstate_StartRun", "document_ExtractText", "document_ExtractText", "gateway_Embed", "gateway_Embed"} {
		if _, err := wrapped(context.Background(), tool, "{}"); err != nil {
			t.Fatalf("wrapped tool %s: %v", tool, err)
		}
	}

	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "indexer_UpsertDocument"},
	}})
	if !retry || !strings.Contains(instruction, "indexer_UpsertChunk") {
		t.Fatalf("guard instruction = %q retry=%t", instruction, retry)
	}
	if _, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{
			Name:      "indexer_UpsertChunk",
			Arguments: validCanonicalUpsertArgs(),
		},
	}}); retry {
		t.Fatal("guard should allow the current missing index step")
	}
	if len(events) == 0 || events[len(events)-1].Type != "blocked_tool_calls" {
		t.Fatalf("events = %+v", events)
	}
}

func TestTrackerBlocksIncompleteCanonicalKnowledgeUpserts(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 1 document.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, step := range []struct {
		name string
		args string
	}{
		{"runstate_StartRun", `{"run":{"items":[{"id":"s1"}]}}`},
		{"document_ExtractText", `{}`},
		{"gateway_Embed", `{}`},
	} {
		if _, err := wrapped(context.Background(), step.name, step.args); err != nil {
			t.Fatalf("wrapped tool %s: %v", step.name, err)
		}
	}

	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{
			Name: "indexer_UpsertChunk",
			Arguments: `{
				"chunkId": "chunk-1",
				"textContentRef": "content_1",
				"embeddingRef": "emb_1",
				"document": {"id": "doc-1"},
				"sourceMetadata": {"name": "source.pdf"},
				"provenance": {"sourceUri": "/tmp/source.pdf"},
				"facts": [{"subject": "source.pdf", "predicate": "contains", "object": "evidence"}],
				"entities": [{"id": "doc-1", "name": "source.pdf", "type": "document"}],
				"relations": [],
				"citations": []
			}`,
		},
	}})
	if !retry || !strings.Contains(instruction, "non-empty citations") {
		t.Fatalf("guard instruction = %q retry=%t", instruction, retry)
	}
}

func TestTrackerBlocksFutureKnowledgeIndexCallsBeforeCurrentStep(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	if _, err := wrapped(context.Background(), "runstate_StartRun", `{"run":{"items":[{"id":"s1"},{"id":"s2"}]}}`); err != nil {
		t.Fatalf("start run: %v", err)
	}

	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "gateway_Embed"},
	}})
	if !retry || !strings.Contains(instruction, "document content extraction") {
		t.Fatalf("guard instruction = %q retry=%t", instruction, retry)
	}
	instruction, retry = tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "io_Read"},
	}})
	if !retry || !strings.Contains(instruction, "document content extraction") {
		t.Fatalf("io_Read should be blocked after runstate starts so extraction uses document services: %q retry=%t", instruction, retry)
	}
}

func TestTrackerAllowsOrderedBatchAcrossKnowledgeIndexSteps(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	calls := []plugin.ToolCall{
		{Function: plugin.ToolCallFunction{Name: "runstate_StartRun", Arguments: `{"items":[{"name":"a.md"},{"name":"b.md"}]}`}},
		{Function: plugin.ToolCallFunction{Name: "document_ExtractText"}},
		{Function: plugin.ToolCallFunction{Name: "document_ExtractText"}},
		{Function: plugin.ToolCallFunction{Name: "gateway_Embed"}},
		{Function: plugin.ToolCallFunction{Name: "gateway_Embed"}},
		{Function: plugin.ToolCallFunction{Name: "indexer_UpsertChunk"}},
		{Function: plugin.ToolCallFunction{Name: "indexer_UpsertChunk"}},
		{Function: plugin.ToolCallFunction{Name: "runstate_MarkComplete"}},
	}
	if instruction, retry := tracker.GuardToolCalls("", calls); retry {
		t.Fatalf("ordered batch should be allowed: %q", instruction)
	}
}

func TestTrackerBlocksUnorderedBatchAcrossKnowledgeIndexSteps(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{
		{Function: plugin.ToolCallFunction{Name: "runstate_StartRun", Arguments: `{"items":[{"name":"a.md"},{"name":"b.md"}]}`}},
		{Function: plugin.ToolCallFunction{Name: "gateway_Embed"}},
	})
	if !retry || !strings.Contains(instruction, "document content extraction") {
		t.Fatalf("guard instruction = %q retry=%t", instruction, retry)
	}
}

func TestTrackerBlocksStartRunWithoutItems(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "runstate_StartRun", Arguments: `{"title":"missing items"}`},
	}})
	if !retry || !strings.Contains(instruction, "document items") {
		t.Fatalf("guard instruction = %q retry=%t", instruction, retry)
	}
	if _, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "runstate_StartRun", Arguments: `{"items":[{"name":"a.md"}]}`},
	}}); retry {
		t.Fatal("valid runstate start should not be blocked")
	}
}

func TestStoreReturnsIndependentStateCopies(t *testing.T) {
	store := NewStore()
	states := store.Begin("session-1", "inspect system metrics", []Intent{{
		Kind:  KindSystemInspect,
		Steps: []Step{{ID: "metrics", Label: "metrics", AnyOf: []string{"system_GetMetrics"}}},
	}})
	if len(states) != 1 {
		t.Fatalf("states = %+v", states)
	}
	states[0].Steps[0].AnyOf[0] = "mutated"
	states[0].Steps[0].CompletedCount = 99

	persisted := store.List("session-1")
	if got := persisted[0].Steps[0].AnyOf[0]; got != "system_GetMetrics" {
		t.Fatalf("stored tool name = %q", got)
	}
	if got := persisted[0].Steps[0].CompletedCount; got != 0 {
		t.Fatalf("stored completion count = %d", got)
	}
}

func TestTrackerEmitsCorrelatedTransitionEvents(t *testing.T) {
	var events []Event
	tracker := NewTracker("session-1", "Search the indexed PDFs for evidence.", toolSchemas(
		"gateway_Embed",
		"indexer_QueryContext",
	), NewStore(), func(event Event) {
		events = append(events, event)
	})
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success":true}`, nil
	})
	if _, err := wrapped(context.Background(), "gateway_Embed", `{}`); err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Type != "detected" || events[1].Type != "step_completed" {
		t.Fatalf("event types = %+v", events)
	}
	if events[0].WorkflowID == "" || events[1].WorkflowID != events[0].WorkflowID ||
		events[1].SessionID != "session-1" || events[1].Tool != "gateway_Embed" {
		t.Fatalf("correlation fields = %+v", events)
	}
}

func TestTrackerCancellationDoesNotAdvanceState(t *testing.T) {
	store := NewStore()
	tracker := NewTracker("session-1", "Search the indexed PDFs for evidence.", toolSchemas(
		"gateway_Embed",
		"indexer_QueryContext",
	), store, nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(ctx context.Context, _, _ string) (string, error) {
		return "", ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := wrapped(ctx, "gateway_Embed", `{}`); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	states := store.List("session-1")
	if got := states[0].Steps[0].CompletedCount; got != 0 {
		t.Fatalf("completed count = %d, want 0", got)
	}
}

func toolSchemas(names ...string) []plugin.ToolSchema {
	out := make([]plugin.ToolSchema, 0, len(names))
	for _, name := range names {
		out = append(out, plugin.ToolSchema{Name: name})
	}
	return out
}

func validCanonicalUpsertArgs() string {
	return `{
		"chunkId": "chunk-1",
		"textContentRef": "content_1",
		"embeddingRef": "emb_1",
		"document": {"id": "doc-1", "name": "source.pdf", "type": "pdf", "sourceUri": "/tmp/source.pdf"},
		"sourceMetadata": {"name": "source.pdf", "sourceUri": "/tmp/source.pdf"},
		"provenance": {"sourceUri": "/tmp/source.pdf", "producedBy": "Quark Knowledge"},
		"facts": [{"subject": "source.pdf", "predicate": "contains", "object": "evidence"}],
		"entities": [{"id": "doc-1", "name": "source.pdf", "type": "document"}],
		"relations": [],
		"citations": [{"id": "cite-1", "sourceUri": "/tmp/source.pdf", "textSpan": "evidence", "confidence": 0.9}]
	}`
}

func stepsByID(steps []Step) map[string]Step {
	out := make(map[string]Step, len(steps))
	for _, step := range steps {
		out[step.ID] = step
	}
	return out
}
