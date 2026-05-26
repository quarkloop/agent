package runstatesvc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/boundary"
	runstatev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/runstate/v1"
)

func TestStartRunPersistsGenericItemsAndReloads(t *testing.T) {
	root := t.TempDir()
	srv := newTestServer(t, root)
	run := startFixtureRun(t, srv).GetRun()
	if run.GetId() == "" || run.GetKind() != "knowledge.index" {
		t.Fatalf("run = %#v", run)
	}
	if got := run.GetStatus(); got != runstatev1.RunStatus_RUN_STATUS_RUNNING {
		t.Fatalf("run status = %s, want running", got)
	}
	if len(run.GetItems()) != 2 || run.GetRetentionExpiresAt() == "" {
		t.Fatalf("items/retention missing: %#v", run)
	}
	reloaded, err := New(root)
	if err != nil {
		t.Fatalf("reload server: %v", err)
	}
	got, err := reloaded.GetRun(context.Background(), &runstatev1.GetRunRequest{RunId: run.GetId()})
	if err != nil || len(got.GetRun().GetItems()) != 2 {
		t.Fatalf("get reloaded run = %#v, %v", got, err)
	}
}

func TestUpdateItemArtifactsReferencesIncompleteAndResume(t *testing.T) {
	srv := newTestServer(t, t.TempDir())
	run := startFixtureRun(t, srv).GetRun()
	itemID := run.GetItems()[0].GetId()
	updated, err := srv.UpdateItemState(context.Background(), &runstatev1.UpdateItemStateRequest{
		RunId: run.GetId(), ItemId: itemID, Phase: "extract",
		Status: runstatev1.RunStatus_RUN_STATUS_SUCCEEDED, ArtifactRef: "artifact://document/text",
		Metadata: map[string]string{"parser": "document"}, ServiceCallRefs: []string{"svc-ref-extract-1"},
	})
	if err != nil || len(updated.GetItem().GetPhases()) != 1 {
		t.Fatalf("update item = %#v, %v", updated, err)
	}
	_, err = srv.AppendArtifact(context.Background(), &runstatev1.AppendArtifactRequest{
		RunId: run.GetId(), ItemId: itemID,
		Artifact: &runstatev1.Artifact{Ref: "artifact://structure", Kind: "structure"},
	})
	if err != nil {
		t.Fatalf("append artifact: %v", err)
	}
	ref, err := srv.AppendReference(context.Background(), &runstatev1.AppendReferenceRequest{
		RunId: run.GetId(), ItemId: itemID,
		Reference: &runstatev1.Reference{Ref: "svc-ref-index-2", Kind: "service_call"},
	})
	if err != nil || ref.GetReference().GetItemId() != itemID || ref.GetReference().GetRef() != "svc-ref-index-2" {
		t.Fatalf("append reference = %#v, %v", ref, err)
	}
	failed, err := srv.MarkFailed(context.Background(), &runstatev1.MarkFailedRequest{
		RunId: run.GetId(), ItemId: itemID, Phase: "embed", Error: "provider rejected request",
	})
	if err != nil || failed.GetRun().GetItems()[0].GetRetryCount() != 1 {
		t.Fatalf("mark failed = %#v, %v", failed, err)
	}
	incomplete, err := srv.ListIncompleteItems(context.Background(), &runstatev1.ListIncompleteItemsRequest{RunId: run.GetId()})
	if err != nil || len(incomplete.GetItems()) != 2 {
		t.Fatalf("incomplete = %#v, %v", incomplete, err)
	}
	resumed, err := srv.ResumeRun(context.Background(), &runstatev1.ResumeRunRequest{RunId: run.GetId()})
	if err != nil || resumed.GetRun().GetItems()[0].GetStatus() != runstatev1.RunStatus_RUN_STATUS_PENDING {
		t.Fatalf("resume = %#v, %v", resumed, err)
	}
	artifacts, err := srv.ListArtifacts(context.Background(), &runstatev1.ListArtifactsRequest{RunId: run.GetId(), ItemId: itemID})
	if err != nil || len(artifacts.GetArtifacts()) != 2 {
		t.Fatalf("artifacts = %#v, %v", artifacts, err)
	}
}

func TestCompletionCancellationAndLeaseOwnership(t *testing.T) {
	srv := newTestServer(t, t.TempDir())
	run := startFixtureRun(t, srv).GetRun()
	leased, err := srv.AcquireLease(context.Background(), &runstatev1.AcquireLeaseRequest{RunId: run.GetId(), OwnerId: "runtime-1", TtlSeconds: 60})
	if err != nil || leased.GetLease().GetRevision() == 0 {
		t.Fatalf("acquire lease = %#v, %v", leased, err)
	}
	_, err = srv.AcquireLease(context.Background(), &runstatev1.AcquireLeaseRequest{RunId: run.GetId(), OwnerId: "runtime-2", TtlSeconds: 60})
	if !boundary.IsCategory(err, boundary.Conflict) {
		t.Fatalf("conflicting lease error = %v", err)
	}
	renewed, err := srv.RenewLease(context.Background(), &runstatev1.RenewLeaseRequest{Key: leased.GetLease().GetKey(), OwnerId: "runtime-1", Revision: leased.GetLease().GetRevision(), TtlSeconds: 120})
	if err != nil || renewed.GetLease().GetRevision() == leased.GetLease().GetRevision() {
		t.Fatalf("renew lease = %#v, %v", renewed, err)
	}
	if _, err := srv.ReleaseLease(context.Background(), &runstatev1.ReleaseLeaseRequest{Key: renewed.GetLease().GetKey(), OwnerId: "runtime-1", Revision: renewed.GetLease().GetRevision()}); err != nil {
		t.Fatalf("release lease: %v", err)
	}
	completed, err := srv.MarkComplete(context.Background(), &runstatev1.MarkCompleteRequest{RunId: run.GetId(), ServiceCallRefs: []string{"svc-ref-complete"}})
	if err != nil || completed.GetRun().GetStatus() != runstatev1.RunStatus_RUN_STATUS_SUCCEEDED {
		t.Fatalf("complete = %#v, %v", completed, err)
	}
	for _, item := range completed.GetRun().GetItems() {
		if item.GetStatus() != runstatev1.RunStatus_RUN_STATUS_SUCCEEDED {
			t.Fatalf("completed run has non-terminal item: %#v", item)
		}
	}
	if _, err := srv.ResumeRun(context.Background(), &runstatev1.ResumeRunRequest{RunId: run.GetId()}); !boundary.IsCategory(err, boundary.Conflict) {
		t.Fatalf("resume completed error = %v", err)
	}
}

func TestRunStateServiceDoesNotCallOtherServices(t *testing.T) {
	root := filepath.Join("..", "..")
	banned := []string{
		"New" + "DocumentServiceClient",
		"New" + "GatewayServiceClient",
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
				t.Fatalf("runstate service must not call another service; found %s in %s", needle, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk runstate service: %v", err)
	}
}

func TestRecordStoreRecreatesMissingStateFile(t *testing.T) {
	root := t.TempDir()
	srv := newTestServer(t, root)
	if err := os.Remove(filepath.Join(root, "runstate-records.json")); err != nil {
		t.Fatalf("remove state: %v", err)
	}
	if run := startFixtureRun(t, srv).GetRun(); run.GetId() == "" {
		t.Fatalf("run was not persisted: %#v", run)
	}
	if _, err := os.Stat(filepath.Join(root, "runstate-records.json")); err != nil {
		t.Fatalf("state file was not recreated: %v", err)
	}
}

func newTestServer(t *testing.T, root string) *Server {
	t.Helper()
	srv, err := New(root, newMemoryLeaseStore())
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	srv.now = func() time.Time { return now }
	return srv
}

func startFixtureRun(t *testing.T, srv *Server) *runstatev1.StartRunResponse {
	t.Helper()
	resp, err := srv.StartRun(context.Background(), &runstatev1.StartRunRequest{
		Space: "test-space", Title: "Knowledge import", Kind: "knowledge.index", ActorRef: "agent:quark-knowledge",
		Items: []*runstatev1.ItemInput{
			{Kind: "document", ResourceUri: "file:///tmp/paper.pdf", Name: "paper.pdf", ContentHash: "sha256:paper"},
			{Kind: "document", ResourceUri: "file:///tmp/resume.pdf", Name: "resume.pdf", ContentHash: "sha256:resume"},
		},
		Metadata: map[string]string{"agent": "quark-knowledge"},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	return resp
}
