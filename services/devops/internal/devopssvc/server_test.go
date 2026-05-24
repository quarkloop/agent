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

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
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

	explained, err := server.ExplainFailure(context.Background(), &devopsv1.ExplainFailureRequest{Logs: "panic: boom\nok"})
	if err != nil {
		t.Fatalf("explain failure: %v", err)
	}
	if len(explained.GetEvidence()) != 1 || !strings.Contains(explained.GetEvidence()[0], "panic") {
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
