package devopssvc

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/boundary"
	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/services/devops/internal/buildrelease"
)

func TestRepoStatusDiffAndReleaseNotes(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\nupdated\n")

	server := NewServer()
	statusResp, err := server.Status(context.Background(), &devopsv1.StatusRequest{Path: repo})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if statusResp.GetClean() || len(statusResp.GetChanges()) == 0 {
		t.Fatalf("expected dirty repo status: %+v", statusResp)
	}

	diffResp, err := server.Diff(context.Background(), &devopsv1.DiffRequest{Path: repo, Files: []string{"README.md"}})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !strings.Contains(diffResp.GetDiff(), "updated") {
		t.Fatalf("diff missing update:\n%s", diffResp.GetDiff())
	}

	notes, err := server.GenerateReleaseNotes(context.Background(), &devopsv1.GenerateReleaseNotesRequest{Path: repo})
	if err != nil {
		t.Fatalf("release notes: %v", err)
	}
	if !strings.Contains(notes.GetMarkdown(), "initial commit") {
		t.Fatalf("release notes missing commit: %+v", notes)
	}

	branch, err := server.GetBranch(context.Background(), &devopsv1.GetBranchRequest{Path: repo})
	if err != nil || branch.GetCurrent() == "" {
		t.Fatalf("branch: response=%+v error=%v", branch, err)
	}
	changed, err := server.ListChangedFiles(context.Background(), &devopsv1.ListChangedFilesRequest{Path: repo})
	if err != nil || len(changed.GetChanges()) == 0 {
		t.Fatalf("changed files: response=%+v error=%v", changed, err)
	}
	commit, err := server.Commit(context.Background(), &devopsv1.CommitRequest{Path: repo, Files: []string{"README.md"}, Message: "update", Reason: "prepare"})
	if err != nil || commit.GetPlan().GetAction() != "repo.commit" {
		t.Fatalf("commit plan: response=%+v error=%v", commit, err)
	}
}

func TestProjectTasksPolicyAndPlans(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.26\n")

	server := NewServer()
	project, err := server.DetectProject(context.Background(), &devopsv1.DetectProjectRequest{Path: dir})
	if err != nil {
		t.Fatalf("detect project: %v", err)
	}
	if project.GetProject().GetKind() != "go" {
		t.Fatalf("project = %+v, want go", project.GetProject())
	}

	resolved, err := server.ResolveTask(context.Background(), &devopsv1.ResolveTaskRequest{Path: dir, Task: "test"})
	if err != nil {
		t.Fatalf("resolve task: %v", err)
	}
	if !strings.Contains(resolved.GetTask().GetCommand(), "go test") {
		t.Fatalf("resolved task = %+v", resolved.GetTask())
	}

	run, err := server.RunTask(context.Background(), &devopsv1.RunTaskRequest{Path: dir, Task: "test", DryRun: true, Reason: "verify"})
	if err != nil {
		t.Fatalf("run task dry-run: %v", err)
	}
	if run.GetPlan().GetApprovalRequired() != true || run.GetResult().GetStatus() != taskStatusPlanned {
		t.Fatalf("dry-run task response = %+v", run)
	}

	policy, err := server.EvaluateChange(context.Background(), &devopsv1.EvaluateChangeRequest{
		Path:   dir,
		Action: "commit",
		Files:  []string{"main.go"},
	})
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if !policy.GetAllowed() || len(policy.GetRequiredApprovals()) != 1 || policy.GetRequiredApprovals()[0] != "repo.commit" {
		t.Fatalf("policy response = %+v", policy)
	}

	patch, err := server.ApplyPatch(context.Background(), &devopsv1.ApplyPatchRequest{
		Path:   dir,
		Patch:  "--- a/main.go\n+++ b/main.go\n@@ -0,0 +1 @@\n+package main\n",
		Reason: "prepare code",
	})
	if err != nil {
		t.Fatalf("apply patch plan: %v", err)
	}
	if !patch.GetPlan().GetApprovalRequired() || len(patch.GetExpectedChanges()) != 1 {
		t.Fatalf("patch plan = %+v", patch)
	}

	artifact, err := server.CreateArtifact(context.Background(), &devopsv1.CreateArtifactRequest{Path: dir, Task: "build", Reason: "produce"})
	if err != nil || len(artifact.GetArtifacts()) != 1 || !strings.HasSuffix(artifact.GetArtifacts()[0].GetUri(), "dist") {
		t.Fatalf("artifact plan: response=%+v error=%v", artifact, err)
	}
}

func TestTestsContainersDeployAndFailureExplanation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.26\n")

	server := NewServer()
	tests, err := server.DiscoverTests(context.Background(), &devopsv1.DiscoverTestsRequest{Path: dir})
	if err != nil {
		t.Fatalf("discover tests: %v", err)
	}
	if len(tests.GetTests()) == 0 {
		t.Fatalf("expected test targets: %+v", tests)
	}

	run, err := server.RunTests(context.Background(), &devopsv1.RunTestsRequest{Path: dir, DryRun: true})
	if err != nil {
		t.Fatalf("run tests dry-run: %v", err)
	}
	if run.GetResult().GetStatus() != taskStatusPlanned {
		t.Fatalf("dry-run tests = %+v", run)
	}

	explained, err := server.ExplainFailure(context.Background(), &devopsv1.ExplainFailureRequest{Evidence: []string{"--- FAIL: TestBroken", "panic: boom"}})
	if err != nil {
		t.Fatalf("explain failure: %v", err)
	}
	if len(explained.GetEvidence()) != 2 || !strings.Contains(explained.GetEvidence()[0], "TestBroken") {
		t.Fatalf("failure explanation = %+v", explained)
	}

	imagePlan, err := server.BuildImage(context.Background(), &devopsv1.BuildImageRequest{Path: dir, Tag: "example:test", DryRun: true})
	if err != nil {
		t.Fatalf("build image plan: %v", err)
	}
	if !imagePlan.GetPlan().GetApprovalRequired() || imagePlan.GetImage().GetName() != "example:test" {
		t.Fatalf("image plan = %+v", imagePlan)
	}

	runPlan, err := server.PlanRun(context.Background(), &devopsv1.PlanRunRequest{Image: "example:test"})
	if err != nil {
		t.Fatalf("plan run: %v", err)
	}
	if runPlan.GetPlan().GetAction() != "container.run" {
		t.Fatalf("run plan = %+v", runPlan)
	}

	deployPlan, err := server.Plan(context.Background(), &devopsv1.PlanRequest{Path: dir, Environment: "staging"})
	if err != nil {
		t.Fatalf("deploy plan: %v", err)
	}
	if deployPlan.GetPlan().GetAction() != "deploy.plan" {
		t.Fatalf("deploy plan = %+v", deployPlan)
	}

	if _, err := server.Apply(context.Background(), &devopsv1.ApplyRequest{PlanId: "plan"}); err == nil {
		t.Fatal("expected approval error for deployment apply")
	}
	applied, err := server.Apply(context.Background(), &devopsv1.ApplyRequest{PlanId: "plan", ApprovalId: "approved"})
	if err != nil || applied.GetResult().GetStatus() != taskStatusPlanned {
		t.Fatalf("approved deployment response=%+v error=%v", applied, err)
	}
}

func TestRunTestsReturnsBoundedFailureEvidenceWithoutRawAggregateLogs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.26\n")
	lines := []string{"--- FAIL: TestBroken (0.00s)", "main_test.go:7: expected stable behavior"}
	for i := 0; i < maxEvidenceLines+5; i++ {
		lines = append(lines, "diagnostic line")
	}
	server := newServer(localWorkspace{}, stubCommands{result: &devopsv1.TaskResult{
		Status:   taskStatusFailed,
		ExitCode: 1,
		Summary:  "go failed",
		Logs:     lines,
	}}, buildrelease.NewRunner())

	run, err := server.RunTests(context.Background(), &devopsv1.RunTestsRequest{Path: dir})
	if err != nil {
		t.Fatalf("run tests: %v", err)
	}
	if got := run.GetResult().GetLogs(); len(got) != 0 {
		t.Fatalf("aggregate response exposed raw logs: %+v", got)
	}
	evidence := run.GetTests()[0].GetEvidence()
	if len(evidence) != maxEvidenceLines || !strings.Contains(evidence[0], "TestBroken") || !strings.Contains(evidence[1], "expected stable behavior") {
		t.Fatalf("bounded evidence = %+v", evidence)
	}
}

func TestRunTestsRejectsCommandTextAsATargetIDBeforeExecution(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.26\n")
	server := newServer(localWorkspace{}, stubCommands{result: &devopsv1.TaskResult{Status: taskStatusPassed}}, buildrelease.NewRunner())

	_, err := server.RunTests(context.Background(), &devopsv1.RunTestsRequest{Path: dir, Targets: []string{"./..."}})
	assertCategory(t, err, boundary.InvalidArgument)
	if err == nil || !strings.Contains(err.Error(), "available ids: test") {
		t.Fatalf("invalid target diagnostic = %v", err)
	}
}

func TestReleaseFunctionsAreOwnedByDevOpsBuildService(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/releasefixture\n\ngo 1.26\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	writeReleaseConfig(t, dir)

	server := NewServer()
	dryRun, err := server.DryRunRelease(context.Background(), &devopsv1.DryRunReleaseRequest{
		Path:    dir,
		Version: "v1.2.3",
	})
	if err != nil {
		t.Fatalf("dry run release: %v", err)
	}
	if dryRun.GetVersion() != "v1.2.3" || len(dryRun.GetPlanned()) != 1 {
		t.Fatalf("dry run response = %+v", dryRun)
	}
	if dryRun.GetPlanned()[0].GetArchiveName() == "" {
		t.Fatalf("planned artifact missing archive name: %+v", dryRun.GetPlanned()[0])
	}

	release, err := server.RunRelease(context.Background(), &devopsv1.RunReleaseRequest{
		Path:      dir,
		Version:   "v1.2.3",
		SkipTests: true,
		Reason:    "verify release service function",
	})
	if err != nil {
		t.Fatalf("run release: %v", err)
	}
	if !release.GetSuccess() || release.GetPlan().GetAction() != "build.run_release" {
		t.Fatalf("release response = %+v", release)
	}
	if len(release.GetArtifacts()) != 1 || release.GetArtifacts()[0].GetChecksum() == "" {
		t.Fatalf("release artifacts = %+v", release.GetArtifacts())
	}
}

func TestInitReleaseConfigReportsApprovalPlan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	server := NewServer()
	resp, err := server.InitReleaseConfig(context.Background(), &devopsv1.InitReleaseConfigRequest{
		Path:   dir,
		Reason: "prepare release config",
	})
	if err != nil {
		t.Fatalf("init release config: %v", err)
	}
	if !resp.GetCreated() || resp.GetPlan().GetAction() != "build.init_release_config" {
		t.Fatalf("init response = %+v", resp)
	}
	if _, err := os.Stat(resp.GetConfigPath()); err != nil {
		t.Fatalf("release config missing: %v", err)
	}
}

func TestDescriptorPublishesCanonicalDevOpsSubjects(t *testing.T) {
	t.Parallel()
	descriptor := Descriptor(nil)
	if len(descriptor.GetRpcs()) == 0 {
		t.Fatal("descriptor has no service functions")
	}
	for _, rpc := range descriptor.GetRpcs() {
		if rpc.GetOwner() != "devops" || !strings.HasPrefix(rpc.GetSubject(), "svc.devops.v1.") {
			t.Fatalf("rpc does not expose canonical devops route: %+v", rpc)
		}
	}
}

func TestWorkspaceInputsCannotEscapeServiceScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.26\n")
	server := NewServer()

	_, err := server.CreateArtifact(context.Background(), &devopsv1.CreateArtifactRequest{Path: dir, OutputDir: "../outside"})
	assertCategory(t, err, boundary.InvalidArgument)
	_, err = server.Diff(context.Background(), &devopsv1.DiffRequest{Path: dir, Files: []string{"../outside"}})
	assertCategory(t, err, boundary.InvalidArgument)
	_, err = server.BuildImage(context.Background(), &devopsv1.BuildImageRequest{Path: dir, Dockerfile: "../Dockerfile", DryRun: true})
	assertCategory(t, err, boundary.InvalidArgument)
	_, err = server.ApplyPatch(context.Background(), &devopsv1.ApplyPatchRequest{Path: dir, Patch: "--- a/../outside\n+++ b/../outside\n"})
	assertCategory(t, err, boundary.InvalidArgument)
}

func TestCommandCancellationAndDeadlineRemainDiagnosticCategories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.26\n")

	canceled := newServer(localWorkspace{}, stubCommands{gitErr: context.Canceled}, buildrelease.NewRunner())
	_, err := canceled.Status(context.Background(), &devopsv1.StatusRequest{Path: dir})
	assertCategory(t, err, boundary.Canceled)

	timedOut := newServer(localWorkspace{}, stubCommands{runErr: context.DeadlineExceeded}, buildrelease.NewRunner())
	_, err = timedOut.RunTask(context.Background(), &devopsv1.RunTaskRequest{Path: dir, Task: "test"})
	assertCategory(t, err, boundary.Deadline)
}

func TestCommandStartFailurePreservesDiagnosticEvidence(t *testing.T) {
	t.Parallel()
	result, err := (osCommands{}).Run(context.Background(), t.TempDir(), "quark-command-that-does-not-exist")
	if err != nil {
		t.Fatalf("run missing executable: %v", err)
	}
	if result.GetStatus() != taskStatusFailed || len(result.GetLogs()) != 1 || !strings.Contains(result.GetLogs()[0], "executable file not found") {
		t.Fatalf("missing executable evidence = %+v", result)
	}
}

func TestContainerInventoryUsesCommandAdapterOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	commands := stubCommands{result: &devopsv1.TaskResult{
		Status: taskStatusPassed,
		Logs:   []string{"abc123\texample\tlatest\tsha256:one", "def456\tother\tdev\tsha256:two"},
	}}
	server := newServer(localWorkspace{}, commands, buildrelease.NewRunner())
	image, err := server.BuildImage(context.Background(), &devopsv1.BuildImageRequest{Path: dir, Tag: "example:latest"})
	if err != nil || image.GetImage().GetUri() != "example:latest" {
		t.Fatalf("image build: response=%+v error=%v", image, err)
	}
	resp, err := server.ListImages(context.Background(), &devopsv1.ListImagesRequest{Filter: "example"})
	if err != nil || len(resp.GetImages()) != 1 || resp.GetImages()[0].GetId() != "abc123" {
		t.Fatalf("images: response=%+v error=%v", resp, err)
	}
}

type stubCommands struct {
	gitErr error
	runErr error
	result *devopsv1.TaskResult
}

func (s stubCommands) Git(_ context.Context, _ string, _ ...string) (string, error) {
	return "", s.gitErr
}

func (s stubCommands) GitPatchCheck(_ context.Context, _, _ string) error {
	return s.gitErr
}

func (s stubCommands) Run(_ context.Context, _ string, _ string, _ ...string) (*devopsv1.TaskResult, error) {
	if s.result != nil {
		return s.result, nil
	}
	return nil, s.runErr
}

func assertCategory(t *testing.T, err error, category boundary.Category) {
	t.Helper()
	if !boundary.IsCategory(err, category) {
		t.Fatalf("error category = %v, want %s", err, category)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "quark@example.test")
	run(t, dir, "git", "config", "user.name", "Quark Test")
	writeFile(t, filepath.Join(dir, "README.md"), "hello\n")
	run(t, dir, "git", "add", "README.md")
	run(t, dir, "git", "commit", "-m", "initial commit")
	return dir
}

func writeReleaseConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := map[string]any{
		"package_name": "releasefixture",
		"binary_name":  "releasefixture",
		"release_dir":  "dist",
		"checksums":    true,
		"builds": []map[string]any{{
			"name":        "releasefixture",
			"source_path": ".",
			"binary_name": "releasefixture",
			"source_dir":  ".",
		}},
		"targets": []map[string]string{{
			"os":   runtime.GOOS,
			"arch": runtime.GOARCH,
		}},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal release config: %v", err)
	}
	writeFile(t, filepath.Join(dir, "build_release.json"), string(append(data, '\n')))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, string(output))
	}
}
