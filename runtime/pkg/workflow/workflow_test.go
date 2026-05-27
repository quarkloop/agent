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

	status := tracker.ContinuationStatus()
	if strings.Contains(status, "document_ParseBytes") {
		t.Fatalf("content extraction status should not advertise metadata-only parsing:\n%s", status)
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

func TestDevOpsFailureWorkflowCannotTreatDiscoveryAsExecution(t *testing.T) {
	tools := toolSchemas("repo_Status", "test_DiscoverTests", "test_RunTests", "test_ExplainFailure")
	tracker := NewTracker("session-1", "Inspect this repository, run its tests, and explain any failure.", tools, NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected devops workflow")
	}

	assertToolNames(t, tracker.CallableTools(tools), "repo_Status")
	tracker.RecordToolResult("repo_Status", `{}`, `{"clean":true}`)
	assertToolNames(t, tracker.CallableTools(tools), "test_DiscoverTests")
	tracker.RecordToolResult("test_DiscoverTests", `{}`, `{"tests":[{"id":"test"}]}`)
	assertToolNames(t, tracker.CallableTools(tools), "test_RunTests")
	tracker.RecordToolResult("test_RunTests", `{"targets":["test"]}`, `{"tests":[{"id":"test","status":"failed","evidence":["--- FAIL: TestBroken"]}]}`)
	assertToolNames(t, tracker.CallableTools(tools), "test_ExplainFailure")
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

func TestTrackerCallableToolsExposeOnlyCurrentWorkflowStage(t *testing.T) {
	tracker := NewTracker("session-1", "Please index this PDF file.", toolSchemas(
		"io_List",
		"io_Read",
		"runstate_StartRun",
		"document_ExtractText",
		"document_GetPages",
		"gateway_Embed",
		"indexer_UpsertDocument",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
		"citation_RenderReferences",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	assertToolNames(t, tracker.CallableTools(toolSchemas(
		"io_List",
		"io_Read",
		"runstate_StartRun",
		"document_ExtractText",
		"document_GetPages",
		"gateway_Embed",
		"indexer_UpsertDocument",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
		"citation_RenderReferences",
	)), "io_List", "runstate_StartRun")

	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success":true}`, nil
	})
	if _, err := wrapped(context.Background(), "runstate_StartRun", `{"items":[{"id":"source"}]}`); err != nil {
		t.Fatal(err)
	}
	assertToolNames(t, tracker.CallableTools(toolSchemas(
		"io_List", "document_ExtractText", "document_GetPages", "gateway_Embed",
	)), "document_ExtractText", "document_GetPages")
	if _, err := wrapped(context.Background(), "document_GetPages", `{"input":{"sourceUri":"/tmp/source.pdf"}}`); err != nil {
		t.Fatal(err)
	}
	assertToolNames(t, tracker.CallableTools(toolSchemas(
		"document_GetPages", "gateway_Embed", "indexer_UpsertDocument",
	)), "document_GetPages", "gateway_Embed")
	if _, err := wrapped(context.Background(), "gateway_Embed", `{"inputs":[{"content":[{"kind":"CONTENT_KIND_PAGE_REF","ref":"page_1"}],"metadata":{"sourceUri":"/tmp/source.pdf"}}]}`); err != nil {
		t.Fatal(err)
	}
	indexTools := tracker.CallableTools([]plugin.ToolSchema{{Name: "indexer_UpsertChunk", Description: "Store a canonical chunk."}})
	if len(indexTools) != 1 || !strings.Contains(indexTools[0].Description, "independent source records may be returned together") {
		t.Fatalf("canonical index batch guidance missing from tool surface: %+v", indexTools)
	}
}

func TestTrackerStopsOfferingEvidenceRefinementAfterBatchEvidenceIsReady(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 PDF files.", toolSchemas(
		"runstate_StartRun", "document_ExtractText", "document_GetPages", "gateway_Embed",
	), NewStore(), nil)
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
		{"document_GetPages", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_GetPages", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
	} {
		tracker.RecordToolResult(call.name, call.args, `{"success":true}`)
	}
	assertToolNames(t, tracker.CallableTools(toolSchemas(
		"document_GetPages", "gateway_Embed",
	)), "gateway_Embed")
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
	if guard, retry := tracker.FinalGuard("The documents are indexed and ready for questions."); retry || guard != "" {
		t.Fatalf("guard after completion = %q retry=%t", guard, retry)
	}
	if guard, retry := tracker.FinalGuard("The run was created; I am ready to proceed with extraction and indexing."); !retry ||
		!strings.Contains(guard, "completed_index_status_mismatch") {
		t.Fatalf("contradictory completion response was not rejected: %q retry=%t", guard, retry)
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

func TestTrackerRequiresAnswerTurnAfterTerminalStepToolCallCompletes(t *testing.T) {
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
	if _, err := wrapped(context.Background(), "runstate_MarkComplete", "{}"); err != nil {
		t.Fatalf("wrapped terminal tool: %v", err)
	}
	if !tracker.AcceptFinalBeforeToolCalls("Indexed all documents.", terminal) {
		t.Fatal("completed workflow should accept an answer turn before a redundant terminal call")
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

func TestTrackerDoesNotTakeBatchScopeFromRunStateItems(t *testing.T) {
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
			return `{"run":{"items":[{"id":"s1"},{"id":"s2"},{"id":"s3"}]}}`, nil
		}
		return `{"success": true}`, nil
	})
	if _, err := wrapped(context.Background(), "runstate_StartRun", "{}"); err != nil {
		t.Fatalf("start run: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := wrapped(context.Background(), "document_ExtractText", "{}"); err != nil {
			t.Fatalf("extract %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := wrapped(context.Background(), "gateway_Embed", "{}"); err != nil {
			t.Fatalf("embed %d: %v", i, err)
		}
	}
	status := tracker.ContinuationStatus()
	if !strings.Contains(status, `"current_step":"canonical indexing"`) {
		t.Fatalf("run state items improperly expanded requested batch scope: %s", status)
	}
	if strings.Contains(status, `"required_count":3`) {
		t.Fatalf("run state item count became workflow authority: %s", status)
	}
}

func TestTrackerCountsDistinctEmbeddingSourcesForBatchProgress(t *testing.T) {
	var events []Event
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
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
	for _, call := range []struct {
		name string
		args string
	}{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
		{"gateway_Embed", `{"inputs":[{"content":[{"kind":"CONTENT_KIND_CONTENT_REF","ref":"content_1"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`},
		{"gateway_Embed", `{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"alternate summary"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatalf("wrapped tool %s: %v", call.name, err)
		}
	}
	status := tracker.ContinuationStatus()
	if !strings.Contains(status, `"current_step":"embedding generation"`) || !strings.Contains(status, `"completed_count":1`) {
		t.Fatalf("duplicate embedding advanced workflow:\n%s", status)
	}
	if len(events) == 0 || events[len(events)-1].Type != "duplicate_result_ignored" {
		t.Fatalf("events = %+v", events)
	}
	if _, err := wrapped(context.Background(), "gateway_Embed", `{"inputs":[{"content":[{"kind":"CONTENT_KIND_CONTENT_REF","ref":"content_2"}],"metadata":{"sourceUri":"/tmp/b.pdf"}}]}`); err != nil {
		t.Fatal(err)
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"current_step":"canonical indexing"`) {
		t.Fatalf("distinct embedding did not advance workflow:\n%s", status)
	}
}

func TestTrackerCountsDistinctGetPagesSourcesForBatchExtraction(t *testing.T) {
	var events []Event
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_GetPages",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), func(event Event) {
		events = append(events, event)
	})
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{"items":[{"id":"a"},{"id":"b"}]}`},
		{"document_GetPages", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_GetPages", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"current_step":"document content extraction"`) || !strings.Contains(status, `"completed_count":1`) {
		t.Fatalf("duplicate page extraction advanced progress:\n%s", status)
	}
	if len(events) == 0 || events[len(events)-1].Type != "duplicate_result_ignored" {
		t.Fatalf("events = %+v", events)
	}
	if _, err := wrapped(context.Background(), "document_GetPages", `{"input":{"sourceUri":"/tmp/b.pdf"}}`); err != nil {
		t.Fatal(err)
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"current_step":"embedding generation"`) {
		t.Fatalf("distinct GetPages extraction did not advance workflow:\n%s", status)
	}
}

func TestTrackerBlocksRepeatedDocumentExtractionBeforeExecution(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{"items":[{"id":"a"},{"id":"b"}]}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.md"}}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{Function: plugin.ToolCallFunction{
		Name: "document_ExtractText", Arguments: `{"input":{"sourceUri":"/tmp/a.md"}}`,
	}}})
	if !retry || !strings.Contains(instruction, "duplicate_source_operation") {
		t.Fatalf("duplicate extraction was not blocked before execution: %s retry=%t", instruction, retry)
	}
	if _, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{Function: plugin.ToolCallFunction{
		Name: "document_ExtractText", Arguments: `{"input":{"sourceUri":"/tmp/b.md"}}`,
	}}}); retry {
		t.Fatal("unprocessed source extraction was rejected")
	}
}

func TestTrackerAllowsOneBoundedEvidenceRefinementWhileWaitingForEmbedding(t *testing.T) {
	tracker := NewTracker("session-1", "Please index this document.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"document_GetPages",
		"document_ExtractLayout",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{"items":[{"id":"a"}]}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	pageArguments := `{"input":{"sourceUri":"/tmp/a.pdf"}}`
	if instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{
		{Function: plugin.ToolCallFunction{Name: "document_GetPages", Arguments: pageArguments}},
	}); retry {
		t.Fatalf("first bounded page refinement during embedding was blocked: %s", instruction)
	}
	if instruction, retry := tracker.GuardToolCalls("I will inspect one bounded page view before selecting evidence.", []plugin.ToolCall{
		{Function: plugin.ToolCallFunction{Name: "document_GetPages", Arguments: pageArguments}},
	}); retry {
		t.Fatalf("narrated bounded page refinement during embedding was blocked: %s", instruction)
	}
	if _, err := wrapped(context.Background(), "document_GetPages", pageArguments); err != nil {
		t.Fatal(err)
	}
	if instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{
		{Function: plugin.ToolCallFunction{Name: "document_GetPages", Arguments: pageArguments}},
	}); !retry || !strings.Contains(instruction, "duplicate_evidence_refinement") {
		t.Fatalf("duplicate page refinement was not blocked: %s retry=%t", instruction, retry)
	}
	embedArguments := `{"inputs":[{"content":[{"kind":"CONTENT_KIND_PAGE_REF","ref":"page_1"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`
	if instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "gateway_Embed", Arguments: embedArguments},
	}}); retry {
		t.Fatalf("page embedding was blocked: %s", instruction)
	}
	if _, err := wrapped(context.Background(), "gateway_Embed", embedArguments); err != nil {
		t.Fatal(err)
	}
	if instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "document_GetPages", Arguments: `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
	}}); !retry || !strings.Contains(instruction, "tool_call_not_in_current_step") {
		t.Fatalf("evidence read after embedding step was not blocked: %s retry=%t", instruction, retry)
	}
}

func TestTrackerCountsEachDistinctEmbeddingInSuccessfulBatch(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	wrapped := tracker.WrapToolHandler(func(_ context.Context, name, _ string) (string, error) {
		if name == "gateway_Embed" {
			return `{"embeddings":[{"embeddingRef":"emb_1"},{"embeddingRef":"emb_2"}]}`, nil
		}
		return `{"success": true}`, nil
	})
	for _, call := range []struct {
		name string
		args string
	}{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
		{"gateway_Embed", `{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"first"}],"metadata":{"sourceUri":"/tmp/a.pdf"}},{"content":[{"kind":"CONTENT_KIND_TEXT","text":"second"}],"metadata":{"sourceUri":"/tmp/b.pdf"}}]}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"current_step":"canonical indexing"`) {
		t.Fatalf("successful embedding batch did not complete both items:\n%s", status)
	}
}

func TestTrackerCountsDistinctIndexedContentForBatchProgress(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, call := range []struct {
		name string
		args string
	}{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
		{"gateway_Embed", `{"contentRef":"content_1"}`},
		{"gateway_Embed", `{"contentRef":"content_2"}`},
		{"indexer_UpsertChunk", `{"chunkId":"a","textContentRef":"content_1","document":{"sourceUri":"/tmp/a.pdf"}}`},
		{"indexer_UpsertChunk", `{"chunkId":"alternate-a","textContentRef":"content_99","document":{"sourceUri":"/tmp/a.pdf"}}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	status := tracker.ContinuationStatus()
	if !strings.Contains(status, `"current_step":"canonical indexing"`) || !strings.Contains(status, `"completed_count":1`) {
		t.Fatalf("duplicate index record advanced workflow:\n%s", status)
	}
}

func TestTrackerRejectsUnattributedKnowledgeEmbeddingInputs(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 1 document.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{Function: plugin.ToolCallFunction{
		Name: "gateway_Embed", Arguments: `{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"summary"}]}]}`,
	}}})
	if !retry || !strings.Contains(instruction, "missing_embedding_source_identity") || !strings.Contains(instruction, "sourceUri") {
		t.Fatalf("missing source identity was not rejected: %q retry=%t", instruction, retry)
	}
	if _, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{Function: plugin.ToolCallFunction{
		Name: "gateway_Embed", Arguments: `{"inputs":[{"content":[{"kind":"CONTENT_KIND_PAGE_REF","ref":"page_1"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`,
	}}}); retry {
		t.Fatal("attributed embedding input was rejected")
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

	status := tracker.ContinuationStatus()
	for _, want := range []string{`"current_step":"durable run completion"`, "runstate_MarkComplete", `"run_id":"run-123"`} {
		if !strings.Contains(status, want) {
			t.Fatalf("continuation status missing %q:\n%s", want, status)
		}
	}
	required := tracker.RequiredToolContinuation()
	if len(required) != 1 || required[0].Function.Name != "runstate_MarkComplete" ||
		!strings.Contains(required[0].Function.Arguments, `"runId":"run-123"`) {
		t.Fatalf("required terminal continuation = %+v", required)
	}
	if repeated := tracker.RequiredToolContinuation(); len(repeated) != 0 {
		t.Fatalf("terminal continuation reissued before result handling: %+v", repeated)
	}
	if _, err := wrapped(context.Background(), required[0].Function.Name, required[0].Function.Arguments); err != nil {
		t.Fatalf("execute required terminal continuation: %v", err)
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"status":"complete"`) || !strings.Contains(status, `"response_mode":"user_answer_only"`) {
		t.Fatalf("required terminal continuation did not complete workflow: %s", status)
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

	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"status":"complete"`) {
		t.Fatalf("completion status was not reported:\n%s", status)
	}
	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "citation_ResolveSpans"},
	}})
	if !retry || !strings.Contains(instruction, `"reason":"tool_call_after_completion"`) {
		t.Fatalf("extra workflow call was not blocked: %q retry=%t", instruction, retry)
	}
	if instruction, retry := tracker.GuardToolCalls("The answer is ready.", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "citation_VerifyGrounding"},
	}}); retry {
		t.Fatalf("redundant completed-step call with final content should remain compatible: %q", instruction)
	}
}

func TestTrackerRequiresOneInlineEmbeddingAndRetrievalForKnowledgeQuery(t *testing.T) {
	const userQuestion = "Search the indexed PDFs and answer from the documents."
	tracker := NewTracker("session-1", userQuestion, toolSchemas(
		"gateway_Embed",
		"indexer_QueryContext",
		"citation_VerifyGrounding",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	tools := tracker.CallableTools([]plugin.ToolSchema{{Name: "gateway_Embed", Description: "Create embeddings."}})
	if len(tools) != 1 || !strings.Contains(tools[0].Description, "submit only the text parameter") {
		t.Fatalf("query embedding guidance missing from tool surface: %+v", tools)
	}
	parameters := tools[0].Parameters
	properties, ok := parameters["properties"].(map[string]any)
	if !ok || len(properties) != 1 || properties["text"] == nil {
		t.Fatalf("query embedding schema was not narrowed to literal text: %+v", parameters)
	}
	required, ok := parameters["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "text" {
		t.Fatalf("query embedding text parameter not required: %+v", parameters)
	}

	for _, invalid := range []string{
		`{"inputRef":"user_question","inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"What is indexed?"}]}]}`,
		`{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"First part?"}]},{"content":[{"kind":"CONTENT_KIND_TEXT","text":"Second part?"}]}]}`,
		`{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"  "}]}]}`,
	} {
		instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
			Function: plugin.ToolCallFunction{Name: "gateway_Embed", Arguments: invalid},
		}})
		if !retry || !strings.Contains(instruction, "query_embedding_requires_inline_text") {
			t.Fatalf("invalid query embedding was not blocked: %q retry=%t", instruction, retry)
		}
	}

	inlineQuestion := `{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"indexed PDF source evidence requested by the user"}]}]}`
	if instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "gateway_Embed", Arguments: inlineQuestion},
	}}); retry {
		t.Fatalf("valid query embedding was blocked: %q", instruction)
	}
	tracker.RecordToolResult("gateway_Embed", inlineQuestion, `{"success":true,"embeddings":[{"embeddingRef":"emb_1"}]}`)

	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{
		{Function: plugin.ToolCallFunction{Name: "indexer_QueryContext", Arguments: `{"queryVectorRef":"emb_1"}`}},
		{Function: plugin.ToolCallFunction{Name: "indexer_QueryContext", Arguments: `{"queryVectorRef":"emb_1"}`}},
	})
	if !retry || !strings.Contains(instruction, "duplicate_completed_step_call") {
		t.Fatalf("duplicate retrieval batch was not blocked: %q retry=%t", instruction, retry)
	}
}

func TestTrackerRejectsInternalToolMarkupInFinalAnswer(t *testing.T) {
	tracker := NewTracker("session-1", "Search the indexed PDFs and answer from the documents.", toolSchemas(
		"gateway_Embed",
		"indexer_QueryContext",
		"citation_VerifyGrounding",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	for _, name := range []string{"gateway_Embed", "indexer_QueryContext", "citation_VerifyGrounding"} {
		tracker.RecordToolResult(name, `{}`, `{"success":true}`)
	}

	guard, retry := tracker.FinalGuard("Let me verify again. <longcat_tool_call>citation_VerifyGrounding</longcat_tool_call>")
	if !retry || !strings.Contains(guard, "internal_tool_markup_in_final_response") ||
		!strings.Contains(guard, `"response_mode":"user_answer_only"`) {
		t.Fatalf("internal function markup was not rejected: %q retry=%t", guard, retry)
	}
	if guard, retry := tracker.FinalGuard("The retrieved document provides the supported answer."); retry || guard != "" {
		t.Fatalf("clean final answer was rejected: %q retry=%t", guard, retry)
	}
}

func TestTrackerContinuationStatusNamesOnlyCurrentStep(t *testing.T) {
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

	status := tracker.ContinuationStatus()
	for _, want := range []string{
		`"status":"incomplete"`,
		`"current_step":"embedding generation"`,
		`"completed_count":1`,
		`"required_count":3`,
		"gateway_Embed",
	} {
		if !strings.Contains(status, want) {
			t.Fatalf("continuation status missing %q:\n%s", want, status)
		}
	}
	for _, notWant := range []string{"canonical indexing", "indexer_UpsertChunk"} {
		if strings.Contains(status, notWant) {
			t.Fatalf("continuation status should not name future step %q:\n%s", notWant, status)
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
	if !strings.Contains(guard, `"current_step":"durable run creation"`) {
		t.Fatalf("guard did not focus on current step:\n%s", guard)
	}
	if strings.Contains(guard, "canonical indexing") {
		t.Fatalf("guard should not emphasize future steps:\n%s", guard)
	}
}

func TestTrackerPersistsCanonicalChunksWithoutSeparateDocumentStage(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun",
		"document_ExtractText",
		"gateway_Embed",
		"indexer_UpsertDocument",
		"indexer_UpsertChunk",
		"runstate_MarkComplete",
	), NewStore(), nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
		{"gateway_Embed", `{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"a evidence"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`},
		{"gateway_Embed", `{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"b evidence"}],"metadata":{"sourceUri":"/tmp/b.pdf"}}]}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatalf("wrapped tool %s: %v", call.name, err)
		}
	}
	assertToolNames(t, tracker.CallableTools(toolSchemas(
		"indexer_UpsertDocument", "indexer_UpsertChunk", "runstate_MarkComplete",
	)), "indexer_UpsertChunk")
	if instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "document_GetPages", Arguments: `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
	}}); !retry || !strings.Contains(instruction, "do not reread source content") {
		t.Fatalf("canonical chunk persistence reread guidance = %q retry=%t", instruction, retry)
	}
	if _, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{
			Name:      "indexer_UpsertChunk",
			Arguments: strings.ReplaceAll(validCanonicalUpsertArgs(), "/tmp/source.pdf", "/tmp/a.pdf"),
		},
	}}); retry {
		t.Fatal("guard should allow the current missing index step")
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"current_step":"canonical indexing"`) {
		t.Fatalf("workflow did not advance directly to canonical chunk persistence:\n%s", status)
	}
}

func TestTrackerRejectsIncompleteCanonicalChunkRecord(t *testing.T) {
	tracker := NewTracker("session-1", "Please index this document.", toolSchemas(
		"runstate_StartRun", "document_ExtractText", "gateway_Embed",
		"indexer_UpsertChunk", "runstate_MarkComplete",
	), NewStore(), nil)
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/source.pdf"}}`},
		{"gateway_Embed", `{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"evidence"}],"metadata":{"sourceUri":"/tmp/source.pdf"}}]}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{
		Function: plugin.ToolCallFunction{Name: "indexer_UpsertChunk", Arguments: `{"chunkId":"chunk-1"}`},
	}})
	if !retry || !strings.Contains(instruction, "incomplete_canonical_record") || !strings.Contains(instruction, "embeddingRef") {
		t.Fatalf("guard instruction = %q retry=%t", instruction, retry)
	}
}

func TestTrackerContextHistoryRetainsCanonicalWriteEvidenceWhileChunksRemain(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun", "document_ExtractText", "gateway_Embed",
		"indexer_UpsertChunk", "runstate_MarkComplete",
	), NewStore(), nil)
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
		{"gateway_Embed", `{"inputs":[{"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`},
		{"gateway_Embed", `{"inputs":[{"metadata":{"sourceUri":"/tmp/b.pdf"}}]}`},
		{"indexer_UpsertChunk", `{"document":{"sourceUri":"/tmp/a.pdf"}}`},
	} {
		tracker.RecordToolResult(call.name, call.args, `{"success":true}`)
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"current_step":"canonical indexing"`) {
		t.Fatalf("tracker did not reach chunk persistence: %s", status)
	}
	pair := func(id, name string) []plugin.Message {
		return []plugin.Message{
			{Role: "assistant", ToolCalls: []plugin.ToolCall{{ID: id, Function: plugin.ToolCallFunction{Name: name}}}},
			{Role: "tool", ToolCallID: id, Content: `{"success":true}`},
		}
	}
	history := []plugin.Message{{Role: "user", Content: "index files"}}
	history = append(history, pair("run", "runstate_StartRun")...)
	history = append(history, pair("extract", "document_ExtractText")...)
	history = append(history, pair("embed", "gateway_Embed")...)
	history = append(history, pair("chunk", "indexer_UpsertChunk")...)

	compacted := tracker.ContextHistory(history)
	var names []string
	for _, message := range compacted {
		for _, call := range message.ToolCalls {
			names = append(names, call.Function.Name)
		}
	}
	got := strings.Join(names, ",")
	if strings.Contains(got, "runstate_StartRun") {
		t.Fatalf("completed run creation history retained during indexing: %s", got)
	}
	for _, want := range []string{"document_ExtractText", "gateway_Embed", "indexer_UpsertChunk"} {
		if !strings.Contains(got, want) {
			t.Fatalf("required indexing evidence %q omitted: %s", want, got)
		}
	}
}

func TestTrackerContextHistoryDropsCanonicalWritesForTerminalCompletion(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun", "document_ExtractText", "gateway_Embed",
		"indexer_UpsertChunk", "runstate_MarkComplete",
	), NewStore(), nil)
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
		{"gateway_Embed", `{"inputs":[{"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`},
		{"gateway_Embed", `{"inputs":[{"metadata":{"sourceUri":"/tmp/b.pdf"}}]}`},
		{"indexer_UpsertChunk", `{"document":{"sourceUri":"/tmp/a.pdf"}}`},
		{"indexer_UpsertChunk", `{"document":{"sourceUri":"/tmp/b.pdf"}}`},
	} {
		tracker.RecordToolResult(call.name, call.args, `{"success":true}`)
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"current_step":"durable run completion"`) {
		t.Fatalf("tracker did not reach terminal completion: %s", status)
	}
	pair := func(id, name string) []plugin.Message {
		return []plugin.Message{
			{Role: "assistant", ToolCalls: []plugin.ToolCall{{ID: id, Function: plugin.ToolCallFunction{Name: name}}}},
			{Role: "tool", ToolCallID: id, Content: `{"success":true}`},
		}
	}
	history := []plugin.Message{{Role: "user", Content: "index files"}}
	history = append(history, pair("run", "runstate_StartRun")...)
	history = append(history, pair("extract", "document_ExtractText")...)
	history = append(history, pair("embed", "gateway_Embed")...)
	history = append(history, pair("chunk", "indexer_UpsertChunk")...)

	compacted := tracker.ContextHistory(history)
	var names []string
	for _, message := range compacted {
		for _, call := range message.ToolCalls {
			names = append(names, call.Function.Name)
		}
	}
	got := strings.Join(names, ",")
	for _, omitted := range []string{"runstate_StartRun", "indexer_UpsertChunk"} {
		if strings.Contains(got, omitted) {
			t.Fatalf("obsolete terminal completion history %q retained: %s", omitted, got)
		}
	}
	if !strings.Contains(got, "document_ExtractText") || !strings.Contains(got, "gateway_Embed") {
		t.Fatalf("required evidence history omitted: %s", got)
	}

	tracker.RecordToolResult("runstate_MarkComplete", `{}`, `{"success":true}`)
	history = append(history, pair("complete", "runstate_MarkComplete")...)
	compacted = tracker.ContextHistory(history)
	for _, message := range compacted {
		if len(message.ToolCalls) > 0 {
			t.Fatalf("completed indexing workflow retained tool history for acknowledgement turn: %+v", compacted)
		}
	}
}

func TestTrackerContextHistoryRetainsDurableRunEvidenceWhileEmbedding(t *testing.T) {
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun", "document_ExtractText", "gateway_Embed",
		"indexer_UpsertChunk", "runstate_MarkComplete",
	), NewStore(), nil)
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
	} {
		tracker.RecordToolResult(call.name, call.args, `{"success":true}`)
	}
	if status := tracker.ContinuationStatus(); !strings.Contains(status, `"current_step":"embedding generation"`) {
		t.Fatalf("tracker did not reach embedding: %s", status)
	}
	history := []plugin.Message{
		{Role: "assistant", ToolCalls: []plugin.ToolCall{{ID: "run", Function: plugin.ToolCallFunction{Name: "runstate_StartRun"}}}},
		{Role: "tool", ToolCallID: "run", Content: `{"success":true}`},
	}
	compacted := tracker.ContextHistory(history)
	if len(compacted) != len(history) || compacted[0].ToolCalls[0].Function.Name != "runstate_StartRun" {
		t.Fatalf("durable run evidence omitted during embedding: %+v", compacted)
	}
}

func TestTrackerBlocksDuplicateAndUnboundedKnowledgeEmbeddings(t *testing.T) {
	var events []Event
	tracker := NewTracker("session-1", "Please index all 2 documents.", toolSchemas(
		"runstate_StartRun", "document_ExtractText", "gateway_Embed", "indexer_UpsertChunk",
	), NewStore(), func(event Event) {
		events = append(events, event)
	})
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	for _, call := range []struct{ name, args string }{
		{"runstate_StartRun", `{}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/a.pdf"}}`},
		{"document_ExtractText", `{"input":{"sourceUri":"/tmp/b.pdf"}}`},
	} {
		if _, err := wrapped(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	instruction, retry := tracker.GuardToolCalls("", []plugin.ToolCall{{Function: plugin.ToolCallFunction{
		Name: "gateway_Embed", Arguments: `{"inputs":[{"content":[{"kind":"CONTENT_KIND_CONTENT_REF","ref":"content_1"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`,
	}}})
	if !retry || !strings.Contains(instruction, "unbounded_embedding_content") {
		t.Fatalf("whole-document embedding was not rejected: %q retry=%t", instruction, retry)
	}
	if len(events) == 0 || events[len(events)-1].Reason != "unbounded_embedding_content" ||
		events[len(events)-1].Details["function"] != "gateway_Embed" {
		t.Fatalf("blocked event diagnostics = %+v", events)
	}
	instruction, retry = tracker.GuardToolCalls("", []plugin.ToolCall{{Function: plugin.ToolCallFunction{
		Name: "gateway_Embed", Arguments: `{"inputs":[{"content":[{"kind":"CONTENT_KIND_PAGE_REF","ref":"page_1"},{"kind":"CONTENT_KIND_PAGE_REF","ref":"page_2"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`,
	}}})
	if !retry || !strings.Contains(instruction, "unbounded_embedding_content") || !strings.Contains(instruction, "one pageRef") || !strings.Contains(instruction, "one input per source") {
		t.Fatalf("multi-page canonical embedding was not rejected: %q retry=%t", instruction, retry)
	}
	instruction, retry = tracker.GuardToolCalls("", []plugin.ToolCall{{Function: plugin.ToolCallFunction{
		Name: "gateway_Embed", Arguments: `{"inputs":[{"content":[{"kind":"CONTENT_KIND_TEXT","text":"copied PDF text"}],"metadata":{"sourceUri":"file:///tmp/a.pdf"}}]}`,
	}}})
	if !retry || !strings.Contains(instruction, "pdf_embedding_requires_page_reference") || !strings.Contains(instruction, "textContentRef") {
		t.Fatalf("copied PDF text embedding was not rejected: %q retry=%t", instruction, retry)
	}
	duplicate := []plugin.ToolCall{
		{Function: plugin.ToolCallFunction{Name: "gateway_Embed", Arguments: `{"inputs":[{"content":[{"kind":"CONTENT_KIND_PAGE_REF","ref":"page_1"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`}},
		{Function: plugin.ToolCallFunction{Name: "gateway_Embed", Arguments: `{"inputs":[{"content":[{"kind":"CONTENT_KIND_PAGE_REF","ref":"page_2"}],"metadata":{"sourceUri":"/tmp/a.pdf"}}]}`}},
	}
	instruction, retry = tracker.GuardToolCalls("", duplicate)
	if !retry || !strings.Contains(instruction, "duplicate_source_operation") {
		t.Fatalf("duplicate source embedding batch was not rejected: %q retry=%t", instruction, retry)
	}
	if events[len(events)-1].Reason != "duplicate_source_operation" ||
		events[len(events)-1].Details["source_identity"] != "source:/tmp/a.pdf" {
		t.Fatalf("duplicate event diagnostics = %+v", events[len(events)-1])
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
	if !retry || !strings.Contains(instruction, `"reason":"missing_run_items"`) {
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

func assertToolNames(t *testing.T, got []plugin.ToolSchema, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("tool names = %+v, want %v", got, want)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("tool names = %+v, want %v", got, want)
		}
	}
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

func TestWorkflowCompletionIdentityUsesChunkSourceURINotFilenameMetadata(t *testing.T) {
	arguments := `{
		"chunkId": "chunk-1",
		"document": {"sourceUri": "file:///tmp/records/source.pdf"},
		"sourceMetadata": {"filename": "source.pdf"}
	}`
	for i := 0; i < 100; i++ {
		if got := workflowCompletionIdentity("index", "indexer_UpsertChunk", arguments); got != "source:/tmp/records/source.pdf" {
			t.Fatalf("chunk source identity = %q, want canonical source URI", got)
		}
	}
}

func stepsByID(steps []Step) map[string]Step {
	out := make(map[string]Step, len(steps))
	for _, step := range steps {
		out[step.ID] = step
	}
	return out
}
