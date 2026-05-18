package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/plugin"
)

func TestDetectKnowledgeIndexWorkflowFromUserLanguage(t *testing.T) {
	intents := Detect("Please index these PDF files so I can ask questions later.", toolSchemas(
		"ingestion_StartRun",
		"document_ExtractText",
		"embedding_Embed",
		"indexer_UpsertChunk",
		"ingestion_MarkComplete",
	))
	if len(intents) != 1 || intents[0].Kind != KindKnowledgeIndex {
		t.Fatalf("intents = %+v", intents)
	}
	if len(intents[0].Steps) != 5 {
		t.Fatalf("steps = %+v", intents[0].Steps)
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
		"build_release_DryRun",
		"policy_EvaluateChange",
	))
	if len(devops) != 1 || devops[0].Kind != KindDevOps {
		t.Fatalf("devops intents = %+v", devops)
	}
	if len(devops[0].Steps) != 4 {
		t.Fatalf("devops steps = %+v", devops[0].Steps)
	}
}

func TestTrackerBlocksFinalizationUntilRequiredServiceResults(t *testing.T) {
	store := NewStore()
	var events []Event
	tracker := NewTracker("session-1", "Index these documents.", toolSchemas(
		"ingestion_StartRun",
		"document_ExtractText",
		"embedding_Embed",
		"indexer_UpsertChunk",
		"ingestion_MarkComplete",
	), store, func(event Event) {
		events = append(events, event)
	})
	if tracker == nil {
		t.Fatal("expected tracker")
	}

	guard, retry := tracker.FinalGuard("")
	if !retry || !strings.Contains(guard, "document extraction") {
		t.Fatalf("guard = %q retry=%t", guard, retry)
	}

	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"ok": true}`, nil
	})
	for _, tool := range []string{"ingestion_StartRun", "document_ExtractText", "embedding_Embed", "indexer_UpsertChunk", "ingestion_MarkComplete"} {
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

func TestTrackerIgnoresFailedServiceResults(t *testing.T) {
	store := NewStore()
	tracker := NewTracker("session-1", "Find information in the indexed PDFs.", toolSchemas(
		"embedding_Embed",
		"indexer_QueryContext",
	), store, nil)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": false}`, nil
	})
	if _, err := wrapped(context.Background(), "embedding_Embed", "{}"); err != nil {
		t.Fatalf("wrapped tool: %v", err)
	}
	if guard, retry := tracker.FinalGuard(""); !retry || !strings.Contains(guard, "query embedding") {
		t.Fatalf("failed result should not complete workflow: %q retry=%t", guard, retry)
	}
}

func toolSchemas(names ...string) []plugin.ToolSchema {
	out := make([]plugin.ToolSchema, 0, len(names))
	for _, name := range names {
		out = append(out, plugin.ToolSchema{Name: name})
	}
	return out
}
