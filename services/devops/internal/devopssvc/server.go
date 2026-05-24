package devopssvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	taskStatusPlanned = "planned"
	taskStatusPassed  = "passed"
	taskStatusFailed  = "failed"
)

type Server struct {
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) Status(ctx context.Context, req *devopsv1.StatusRequest) (*devopsv1.StatusResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	branch, _ := branchInfo(ctx, root)
	changes, err := gitChanges(ctx, root, true)
	if err != nil {
		return nil, grpcErr(err)
	}
	return &devopsv1.StatusResponse{
		Branch:     branch,
		Clean:      len(changes) == 0,
		Changes:    changes,
		ObservedAt: timestamppb.Now(),
	}, nil
}

func (s *Server) Diff(ctx context.Context, req *devopsv1.DiffRequest) (*devopsv1.DiffResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	args := []string{"diff"}
	if req.GetStaged() {
		args = append(args, "--staged")
	}
	args = append(args, "--")
	files := sanitizedFiles(req.GetFiles())
	args = append(args, files...)
	diff, err := runGit(ctx, root, args...)
	if err != nil {
		return nil, grpcErr(err)
	}
	return &devopsv1.DiffResponse{Diff: diff, Files: files}, nil
}

func (s *Server) GetBranch(ctx context.Context, req *devopsv1.GetBranchRequest) (*devopsv1.GetBranchResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	current, upstream := branchInfo(ctx, root)
	return &devopsv1.GetBranchResponse{Current: current, Upstream: upstream}, nil
}

func (s *Server) ListChangedFiles(ctx context.Context, req *devopsv1.ListChangedFilesRequest) (*devopsv1.ListChangedFilesResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	changes, err := gitChanges(ctx, root, req.GetIncludeUntracked())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &devopsv1.ListChangedFilesResponse{Changes: changes}, nil
}

func (s *Server) ApplyPatch(ctx context.Context, req *devopsv1.ApplyPatchRequest) (*devopsv1.ApplyPatchResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	files := filesFromPatch(req.GetPatch())
	if req.GetDryRun() {
		if strings.TrimSpace(req.GetPatch()) == "" {
			return nil, serviceerrors.InvalidArgument("patch is required")
		}
		cmd := exec.CommandContext(ctx, "git", "-C", root, "apply", "--check")
		cmd.Stdin = strings.NewReader(req.GetPatch())
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, serviceerrors.InvalidArgumentf("patch check failed: %v\n%s", err, string(output))
		}
	}
	return &devopsv1.ApplyPatchResponse{
		Plan:            mutationPlan("repo.apply_patch", root, req.GetReason(), true, "workspace.write"),
		ExpectedChanges: fileChanges(files, "modified"),
	}, nil
}

func (s *Server) Commit(_ context.Context, req *devopsv1.CommitRequest) (*devopsv1.CommitResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	if strings.TrimSpace(req.GetMessage()) == "" {
		return nil, serviceerrors.InvalidArgument("message is required")
	}
	return &devopsv1.CommitResponse{Plan: mutationPlan("repo.commit", root, req.GetReason(), true, "repo.commit")}, nil
}

func (s *Server) GenerateReleaseNotes(ctx context.Context, req *devopsv1.GenerateReleaseNotesRequest) (*devopsv1.GenerateReleaseNotesResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	rangeRef := strings.TrimSpace(req.GetFromRef())
	toRef := strings.TrimSpace(req.GetToRef())
	if toRef != "" {
		if rangeRef != "" {
			rangeRef += ".."
		}
		rangeRef += toRef
	}
	args := []string{"log", "--pretty=format:%h %s"}
	if rangeRef != "" {
		args = append(args, rangeRef)
	} else {
		args = append(args, "-20")
	}
	out, err := runGit(ctx, root, args...)
	if err != nil {
		return nil, grpcErr(err)
	}
	commits := nonEmptyLines(out)
	var b strings.Builder
	b.WriteString("# Release Notes\n")
	for _, commit := range commits {
		fmt.Fprintf(&b, "- %s\n", commit)
	}
	return &devopsv1.GenerateReleaseNotesResponse{Markdown: strings.TrimSpace(b.String()), Commits: commits}, nil
}

func (s *Server) DetectProject(_ context.Context, req *devopsv1.DetectProjectRequest) (*devopsv1.DetectProjectResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &devopsv1.DetectProjectResponse{Project: detectProject(root)}, nil
}

func (s *Server) ResolveTask(_ context.Context, req *devopsv1.ResolveTaskRequest) (*devopsv1.ResolveTaskResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	taskName := strings.TrimSpace(req.GetTask())
	for _, task := range detectProject(root).GetTasks() {
		if task.GetName() == taskName {
			return &devopsv1.ResolveTaskResponse{Task: task}, nil
		}
	}
	return nil, serviceerrors.NotFoundf("task %q not found", taskName)
}

func (s *Server) RunTask(ctx context.Context, req *devopsv1.RunTaskRequest) (*devopsv1.RunTaskResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	taskResp, err := s.ResolveTask(ctx, &devopsv1.ResolveTaskRequest{Path: root, Task: req.GetTask()})
	if err != nil {
		return nil, err
	}
	plan := mutationPlan("build.run_task", taskResp.GetTask().GetName(), req.GetReason(), true, "command.execute")
	if req.GetDryRun() {
		return &devopsv1.RunTaskResponse{Plan: plan, Result: plannedResult(taskResp.GetTask().GetCommand())}, nil
	}
	result := runResolvedCommand(ctx, root, taskResp.GetTask().GetCommand())
	return &devopsv1.RunTaskResponse{Plan: plan, Result: result}, nil
}

func (s *Server) CreateArtifact(_ context.Context, req *devopsv1.CreateArtifactRequest) (*devopsv1.CreateArtifactResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	task := strings.TrimSpace(req.GetTask())
	if task == "" {
		task = "build"
	}
	outputDir := strings.TrimSpace(req.GetOutputDir())
	if outputDir == "" {
		outputDir = filepath.Join(root, "dist")
	}
	artifact := &devopsv1.Artifact{Name: task, Kind: "build-output", Uri: outputDir}
	return &devopsv1.CreateArtifactResponse{
		Plan:      mutationPlan("build.create_artifact", outputDir, req.GetReason(), true, "artifact.write"),
		Artifacts: []*devopsv1.Artifact{artifact},
	}, nil
}

func (s *Server) DiscoverTests(_ context.Context, req *devopsv1.DiscoverTestsRequest) (*devopsv1.DiscoverTestsResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	project := detectProject(root)
	tests := make([]*devopsv1.TestTarget, 0)
	for _, task := range project.GetTasks() {
		if strings.Contains(task.GetName(), "test") {
			tests = append(tests, &devopsv1.TestTarget{Id: task.GetName(), Kind: project.GetKind(), Path: root, Command: task.GetCommand()})
		}
	}
	return &devopsv1.DiscoverTestsResponse{Tests: tests}, nil
}

func (s *Server) RunTests(ctx context.Context, req *devopsv1.RunTestsRequest) (*devopsv1.RunTestsResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	targets := req.GetTargets()
	if len(targets) == 0 {
		targets = []string{"test"}
	}
	results := make([]*devopsv1.TestResult, 0, len(targets))
	overall := &devopsv1.TaskResult{Status: taskStatusPassed, Summary: "all selected test targets passed"}
	for _, target := range targets {
		task, ok := taskByName(detectProject(root), target)
		if !ok {
			results = append(results, &devopsv1.TestResult{Id: target, Status: taskStatusFailed, Summary: "test target not found"})
			overall.Status = taskStatusFailed
			continue
		}
		if req.GetDryRun() {
			results = append(results, &devopsv1.TestResult{Id: target, Status: taskStatusPlanned, Summary: task.GetCommand()})
			continue
		}
		result := runResolvedCommand(ctx, root, task.GetCommand())
		results = append(results, &devopsv1.TestResult{Id: target, Status: result.GetStatus(), Summary: result.GetSummary()})
		if result.GetStatus() != taskStatusPassed {
			overall = result
		}
	}
	if req.GetDryRun() {
		overall = &devopsv1.TaskResult{Status: taskStatusPlanned, Summary: "test targets planned"}
	}
	return &devopsv1.RunTestsResponse{Result: overall, Tests: results}, nil
}

func (s *Server) ExplainFailure(_ context.Context, req *devopsv1.ExplainFailureRequest) (*devopsv1.ExplainFailureResponse, error) {
	lines := failureLines(req.GetLogs())
	summary := "No failure evidence was provided."
	if len(lines) > 0 {
		summary = "The test output contains failure evidence; inspect the listed lines first."
	}
	return &devopsv1.ExplainFailureResponse{
		Summary:            summary,
		Evidence:           lines,
		SuggestedNextSteps: []string{"Re-run the smallest failing target.", "Inspect the first failure before changing production code."},
	}, nil
}

func (s *Server) BuildImage(ctx context.Context, req *devopsv1.BuildImageRequest) (*devopsv1.BuildImageResponse, error) {
	root, err := resolvePath(req.GetPath())
	if err != nil {
		return nil, grpcErr(err)
	}
	dockerfile := firstNonBlank(req.GetDockerfile(), "Dockerfile")
	tag := firstNonBlank(req.GetTag(), filepath.Base(root)+":dev")
	plan := mutationPlan("container.build", tag, req.GetReason(), true, "container.build")
	if req.GetDryRun() {
		return &devopsv1.BuildImageResponse{Plan: plan, Image: &devopsv1.Artifact{Name: tag, Kind: "container-image"}}, nil
	}
	result := runCommand(ctx, root, "docker", "build", "-f", dockerfile, "-t", tag, ".")
	if result.GetStatus() != taskStatusPassed {
		return nil, serviceerrors.Internalf("docker build failed: %s", strings.Join(result.GetLogs(), "\n"))
	}
	return &devopsv1.BuildImageResponse{Plan: plan, Image: &devopsv1.Artifact{Name: tag, Kind: "container-image", Uri: tag}}, nil
}

func (s *Server) ListImages(ctx context.Context, req *devopsv1.ListImagesRequest) (*devopsv1.ListImagesResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 20
	}
	out := runCommand(ctx, "", "docker", "images", "--format", "{{.ID}}\t{{.Repository}}\t{{.Tag}}\t{{.Digest}}")
	if out.GetStatus() != taskStatusPassed {
		return &devopsv1.ListImagesResponse{}, nil
	}
	filter := strings.ToLower(strings.TrimSpace(req.GetFilter()))
	images := make([]*devopsv1.ContainerImage, 0)
	for _, line := range nonEmptyLines(strings.Join(out.GetLogs(), "\n")) {
		if filter != "" && !strings.Contains(strings.ToLower(line), filter) {
			continue
		}
		parts := strings.Split(line, "\t")
		for len(parts) < 4 {
			parts = append(parts, "")
		}
		images = append(images, &devopsv1.ContainerImage{Id: parts[0], Repository: parts[1], Tag: parts[2], Digest: parts[3]})
		if len(images) >= limit {
			break
		}
	}
	return &devopsv1.ListImagesResponse{Images: images}, nil
}

func (s *Server) PlanRun(_ context.Context, req *devopsv1.PlanRunRequest) (*devopsv1.PlanRunResponse, error) {
	if strings.TrimSpace(req.GetImage()) == "" {
		return nil, serviceerrors.InvalidArgument("image is required")
	}
	return &devopsv1.PlanRunResponse{Plan: mutationPlan("container.run", req.GetImage(), "run container with selected args/env", true, "container.run")}, nil
}

func (s *Server) Plan(_ context.Context, req *devopsv1.PlanRequest) (*devopsv1.PlanResponse, error) {
	target := firstNonBlank(req.GetTarget(), req.GetEnvironment(), req.GetPath())
	return &devopsv1.PlanResponse{
		Plan:    mutationPlan("deploy.plan", target, "deployment planning only", true, "deploy.plan"),
		Changes: []string{"deployment apply is not executed during planning"},
	}, nil
}

func (s *Server) Apply(_ context.Context, req *devopsv1.ApplyRequest) (*devopsv1.ApplyResponse, error) {
	if strings.TrimSpace(req.GetApprovalId()) == "" {
		return nil, serviceerrors.PermissionDenied("approval_id is required to apply a deployment plan")
	}
	return &devopsv1.ApplyResponse{Result: &devopsv1.TaskResult{Status: taskStatusPlanned, Summary: "deployment apply adapter is not configured"}}, nil
}

func (s *Server) EvaluateChange(_ context.Context, req *devopsv1.EvaluateChangeRequest) (*devopsv1.EvaluateChangeResponse, error) {
	action := strings.ToLower(strings.TrimSpace(req.GetAction()))
	required := approvalsForAction(action)
	violations := make([]string, 0)
	for _, file := range req.GetFiles() {
		if strings.Contains(file, "..") {
			violations = append(violations, "path traversal is not allowed: "+file)
		}
	}
	return &devopsv1.EvaluateChangeResponse{Allowed: len(violations) == 0, Violations: violations, RequiredApprovals: required}, nil
}

func resolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	return abs, nil
}

func gitChanges(ctx context.Context, root string, includeUntracked bool) ([]*devopsv1.FileChange, error) {
	out, err := runGit(ctx, root, "status", "--porcelain=v1")
	if err != nil {
		return nil, err
	}
	changes := make([]*devopsv1.FileChange, 0)
	for _, line := range nonEmptyLines(out) {
		if len(line) < 4 {
			continue
		}
		statusCode := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		if strings.HasPrefix(statusCode, "??") && !includeUntracked {
			continue
		}
		changes = append(changes, &devopsv1.FileChange{
			Path:   path,
			Status: gitStatusName(statusCode),
			Staged: line[0] != ' ' && line[0] != '?',
		})
	}
	return changes, nil
}

func branchInfo(ctx context.Context, root string) (string, string) {
	current, _ := runGit(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	upstream, _ := runGit(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	return strings.TrimSpace(current), strings.TrimSpace(upstream)
}

func runGit(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

func detectProject(root string) *devopsv1.Project {
	project := &devopsv1.Project{Kind: "generic", Root: root}
	addBuildFile := func(name string) bool {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			project.BuildFiles = append(project.BuildFiles, name)
			return true
		}
		return false
	}
	if addBuildFile("go.work") || addBuildFile("go.mod") {
		project.Kind = "go"
		project.Tasks = append(project.Tasks,
			&devopsv1.BuildTask{Name: "test", Command: "go test ./...", Inputs: []string{"*.go", "go.mod", "go.work"}},
			&devopsv1.BuildTask{Name: "build", Command: "go build ./...", Inputs: []string{"*.go", "go.mod", "go.work"}},
		)
	}
	if addBuildFile("Makefile") {
		project.Tasks = append(project.Tasks,
			&devopsv1.BuildTask{Name: "make-test", Command: "make test", Inputs: []string{"Makefile"}},
			&devopsv1.BuildTask{Name: "make-build", Command: "make build", Inputs: []string{"Makefile"}, Outputs: []string{"bin"}},
		)
	}
	if addBuildFile("package.json") {
		if project.Kind == "generic" {
			project.Kind = "node"
		}
		project.Tasks = append(project.Tasks, &devopsv1.BuildTask{Name: "npm-test", Command: "npm test", Inputs: []string{"package.json"}})
	}
	if addBuildFile("Dockerfile") {
		project.Tasks = append(project.Tasks, &devopsv1.BuildTask{Name: "docker-build-plan", Command: "docker build .", Inputs: []string{"Dockerfile"}, Outputs: []string{"container-image"}, MutatesWorkspace: false})
	}
	return project
}

func taskByName(project *devopsv1.Project, name string) (*devopsv1.BuildTask, bool) {
	for _, task := range project.GetTasks() {
		if task.GetName() == name {
			return task, true
		}
	}
	return nil, false
}

func runResolvedCommand(ctx context.Context, root, command string) *devopsv1.TaskResult {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return &devopsv1.TaskResult{Status: taskStatusFailed, ExitCode: 1, Summary: "empty command"}
	}
	return runCommand(ctx, root, parts[0], parts[1:]...)
}

func runCommand(ctx context.Context, root, name string, args ...string) *devopsv1.TaskResult {
	cmd := exec.CommandContext(ctx, name, args...)
	if root != "" {
		cmd.Dir = root
	}
	out, err := cmd.CombinedOutput()
	logs := nonEmptyLines(string(out))
	if err != nil {
		exitCode := int32(1)
		if cmd.ProcessState != nil {
			exitCode = int32(cmd.ProcessState.ExitCode())
		}
		return &devopsv1.TaskResult{Status: taskStatusFailed, ExitCode: exitCode, Summary: fmt.Sprintf("%s failed", name), Logs: logs}
	}
	return &devopsv1.TaskResult{Status: taskStatusPassed, ExitCode: 0, Summary: fmt.Sprintf("%s completed", name), Logs: logs}
}

func plannedResult(command string) *devopsv1.TaskResult {
	return &devopsv1.TaskResult{Status: taskStatusPlanned, Summary: command}
}

func mutationPlan(action, target, reason string, approvalRequired bool, risks ...string) *devopsv1.MutationPlan {
	idSeed := action + "|" + target + "|" + reason + "|" + strings.Join(risks, ",")
	sum := sha256.Sum256([]byte(idSeed))
	return &devopsv1.MutationPlan{
		Id:               hex.EncodeToString(sum[:8]),
		Action:           action,
		Target:           target,
		Reason:           strings.TrimSpace(reason),
		ApprovalRequired: approvalRequired,
		Risks:            append([]string(nil), risks...),
	}
}

func filesFromPatch(patch string) []string {
	seen := map[string]struct{}{}
	for _, line := range strings.Split(patch, "\n") {
		if !strings.HasPrefix(line, "+++ ") && !strings.HasPrefix(line, "--- ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "--- "))
		path = strings.TrimPrefix(path, "a/")
		path = strings.TrimPrefix(path, "b/")
		if path == "/dev/null" || path == "" {
			continue
		}
		seen[path] = struct{}{}
	}
	files := make([]string, 0, len(seen))
	for file := range seen {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

func fileChanges(files []string, status string) []*devopsv1.FileChange {
	changes := make([]*devopsv1.FileChange, 0, len(files))
	for _, file := range files {
		changes = append(changes, &devopsv1.FileChange{Path: file, Status: status})
	}
	return changes
}

func sanitizedFiles(files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" || strings.Contains(file, "..") || filepath.IsAbs(file) {
			continue
		}
		out = append(out, file)
	}
	sort.Strings(out)
	return out
}

func gitStatusName(statusCode string) string {
	switch {
	case strings.Contains(statusCode, "??"):
		return "untracked"
	case strings.Contains(statusCode, "A"):
		return "added"
	case strings.Contains(statusCode, "D"):
		return "deleted"
	case strings.Contains(statusCode, "R"):
		return "renamed"
	case strings.Contains(statusCode, "M"):
		return "modified"
	default:
		return strings.TrimSpace(statusCode)
	}
}

func approvalsForAction(action string) []string {
	switch {
	case strings.Contains(action, "commit"):
		return []string{"repo.commit"}
	case strings.Contains(action, "patch") || strings.Contains(action, "write"):
		return []string{"workspace.write"}
	case strings.Contains(action, "test") || strings.Contains(action, "build"):
		return []string{"command.execute"}
	case strings.Contains(action, "container"):
		return []string{"container.build"}
	case strings.Contains(action, "deploy"):
		return []string{"deploy.apply"}
	default:
		return nil
	}
}

func failureLines(logs string) []string {
	out := make([]string, 0)
	for _, line := range nonEmptyLines(logs) {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "fail") || strings.Contains(lower, "error") || strings.Contains(lower, "panic") {
			out = append(out, line)
		}
	}
	if len(out) > 20 {
		return out[:20]
	}
	return out
}

func nonEmptyLines(value string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func grpcErr(err error) error {
	if err == nil {
		return nil
	}
	return serviceerrors.InvalidArgument(err.Error())
}
