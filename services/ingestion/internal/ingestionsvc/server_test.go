package ingestionsvc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/boundary"
	ingestionv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/ingestion/v1"
)

func TestStartRunPersistsAndReloads(t *testing.T) {
	root := t.TempDir()
	srv := newTestServer(t, root)

	start := startFixtureRun(t, srv)
	run := start.GetRun()
	if run.GetId() == "" {
		t.Fatalf("run id is empty")
	}
	if got := run.GetStatus(); got != ingestionv1.SourceStatus_SOURCE_STATUS_RUNNING {
		t.Fatalf("run status = %s, want running", got)
	}
	if len(run.GetSources()) != 2 {
		t.Fatalf("sources = %d, want 2", len(run.GetSources()))
	}
	if run.GetSources()[0].GetRetryCount() != 0 {
		t.Fatalf("retry count = %d, want 0", run.GetSources()[0].GetRetryCount())
	}

	reloaded, err := New(root)
	if err != nil {
		t.Fatalf("reload server: %v", err)
	}
	got, err := reloaded.GetRun(context.Background(), &ingestionv1.GetRunRequest{RunId: run.GetId()})
	if err != nil {
		t.Fatalf("get reloaded run: %v", err)
	}
	if got.GetRun().GetId() != run.GetId() || len(got.GetRun().GetSources()) != 2 {
		t.Fatalf("reloaded run mismatch: %#v", got.GetRun())
	}
}

func TestUpdateSourceStateArtifactsIncompleteAndResume(t *testing.T) {
	srv := newTestServer(t, t.TempDir())
	run := startFixtureRun(t, srv).GetRun()
	sourceID := run.GetSources()[0].GetId()

	updated, err := srv.UpdateSourceState(context.Background(), &ingestionv1.UpdateSourceStateRequest{
		RunId:       run.GetId(),
		SourceId:    sourceID,
		Phase:       ingestionv1.SourcePhase_SOURCE_PHASE_PARSED,
		Status:      ingestionv1.SourceStatus_SOURCE_STATUS_SUCCEEDED,
		ArtifactRef: "artifact://document/text",
		Metadata:    map[string]string{"parser": "document"},
	})
	if err != nil {
		t.Fatalf("update source: %v", err)
	}
	if updated.GetSource().GetExtraction().GetStatus() != ingestionv1.SourceStatus_SOURCE_STATUS_SUCCEEDED {
		t.Fatalf("extraction status = %s", updated.GetSource().GetExtraction().GetStatus())
	}

	artifact, err := srv.AppendArtifact(context.Background(), &ingestionv1.AppendArtifactRequest{
		RunId:    run.GetId(),
		SourceId: sourceID,
		Artifact: &ingestionv1.IngestionArtifact{
			Ref:      "artifact://model/structure",
			Kind:     "structure",
			Metadata: map[string]string{"schema": "open"},
		},
	})
	if err != nil {
		t.Fatalf("append artifact: %v", err)
	}
	if artifact.GetArtifact().GetSourceId() != sourceID {
		t.Fatalf("artifact source = %q, want %q", artifact.GetArtifact().GetSourceId(), sourceID)
	}

	failed, err := srv.MarkFailed(context.Background(), &ingestionv1.MarkFailedRequest{
		RunId:    run.GetId(),
		SourceId: sourceID,
		Phase:    ingestionv1.SourcePhase_SOURCE_PHASE_EMBEDDED,
		Error:    "embedding quota",
		Metadata: map[string]string{"provider": "openrouter"},
	})
	if err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	source := failed.GetRun().GetSources()[0]
	if source.GetStatus() != ingestionv1.SourceStatus_SOURCE_STATUS_FAILED || source.GetRetryCount() != 1 {
		t.Fatalf("source after failure = %#v", source)
	}

	incomplete, err := srv.ListIncompleteSources(context.Background(), &ingestionv1.ListIncompleteSourcesRequest{RunId: run.GetId()})
	if err != nil {
		t.Fatalf("list incomplete: %v", err)
	}
	if len(incomplete.GetSources()) != 2 {
		t.Fatalf("incomplete sources = %d, want 2", len(incomplete.GetSources()))
	}

	resumed, err := srv.ResumeRun(context.Background(), &ingestionv1.ResumeRunRequest{RunId: run.GetId()})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if resumed.GetRun().GetSources()[0].GetStatus() != ingestionv1.SourceStatus_SOURCE_STATUS_PENDING {
		t.Fatalf("resumed source status = %s, want pending", resumed.GetRun().GetSources()[0].GetStatus())
	}

	artifacts, err := srv.ListArtifacts(context.Background(), &ingestionv1.ListArtifactsRequest{RunId: run.GetId(), SourceId: sourceID})
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts.GetArtifacts()) != 2 {
		t.Fatalf("artifacts = %d, want 2", len(artifacts.GetArtifacts()))
	}
}

func TestMarkCompleteCancelListRunsAndValidation(t *testing.T) {
	srv := newTestServer(t, t.TempDir())
	run := startFixtureRun(t, srv).GetRun()

	completed, err := srv.MarkComplete(context.Background(), &ingestionv1.MarkCompleteRequest{RunId: run.GetId()})
	if err != nil {
		t.Fatalf("mark complete: %v", err)
	}
	if completed.GetRun().GetStatus() != ingestionv1.SourceStatus_SOURCE_STATUS_SUCCEEDED {
		t.Fatalf("completed status = %s", completed.GetRun().GetStatus())
	}

	_, err = srv.ResumeRun(context.Background(), &ingestionv1.ResumeRunRequest{RunId: run.GetId()})
	if !boundary.IsCategory(err, boundary.Conflict) {
		t.Fatalf("resume completed error = %v, want FailedPrecondition", err)
	}

	second := startFixtureRun(t, srv).GetRun()
	cancelled, err := srv.CancelRun(context.Background(), &ingestionv1.CancelRunRequest{RunId: second.GetId(), Reason: "user stopped"})
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled.GetRun().GetStatus() != ingestionv1.SourceStatus_SOURCE_STATUS_CANCELLED {
		t.Fatalf("cancelled status = %s", cancelled.GetRun().GetStatus())
	}

	listed, err := srv.ListRuns(context.Background(), &ingestionv1.ListRunsRequest{
		Space:  "test-space",
		Status: ingestionv1.SourceStatus_SOURCE_STATUS_SUCCEEDED,
	})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(listed.GetRuns()) != 1 || listed.GetRuns()[0].GetId() != run.GetId() {
		t.Fatalf("listed runs = %#v, want completed run", listed.GetRuns())
	}

	_, err = srv.StartRun(context.Background(), &ingestionv1.StartRunRequest{Space: "test-space"})
	if !boundary.IsCategory(err, boundary.InvalidArgument) {
		t.Fatalf("empty sources error = %v, want InvalidArgument", err)
	}
}

func TestIngestionServiceDoesNotCallOtherServices(t *testing.T) {
	root := filepath.Join("..", "..")
	banned := []string{
		"New" + "DocumentServiceClient",
		"New" + "ModelServiceClient",
		"New" + "EmbeddingServiceClient",
		"New" + "IndexerServiceClient",
		"New" + "CitationServiceClient",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, needle := range banned {
			if strings.Contains(string(data), needle) {
				t.Fatalf("ingestion service must not call another service; found %s in %s", needle, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ingestion service: %v", err)
	}
}

func TestIngestionStoreRecreatesMissingStateFile(t *testing.T) {
	root := t.TempDir()
	srv := newTestServer(t, root)
	if err := os.Remove(filepath.Join(root, "ingestion-state.json")); err != nil {
		t.Fatalf("remove state: %v", err)
	}

	resp := startFixtureRun(t, srv)
	if resp.GetRun().GetId() == "" {
		t.Fatalf("run was not persisted after state recreation: %#v", resp.GetRun())
	}
	if _, err := os.Stat(filepath.Join(root, "ingestion-state.json")); err != nil {
		t.Fatalf("state file was not recreated: %v", err)
	}
}

func newTestServer(t *testing.T, root string) *Server {
	t.Helper()
	srv, err := New(root)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	srv.now = func() time.Time { return now }
	return srv
}

func startFixtureRun(t *testing.T, srv *Server) *ingestionv1.StartRunResponse {
	t.Helper()
	resp, err := srv.StartRun(context.Background(), &ingestionv1.StartRunRequest{
		Space: "test-space",
		Title: "Knowledge import",
		Sources: []*ingestionv1.SourceInput{{
			SourceUri:  "file:///tmp/paper.pdf",
			Filename:   "paper.pdf",
			SourceHash: "sha256:paper",
			FilePath:   "/tmp/paper.pdf",
			Metadata:   map[string]string{"kind": "paper"},
		}, {
			SourceUri:  "file:///tmp/resume.pdf",
			Filename:   "resume.pdf",
			SourceHash: "sha256:resume",
			FilePath:   "/tmp/resume.pdf",
			Metadata:   map[string]string{"kind": "resume"},
		}},
		Metadata: map[string]string{"agent": "quark-knowledge"},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	return resp
}
