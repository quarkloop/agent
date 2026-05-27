package devopssvc

import (
	"context"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

func (s *Server) DiscoverTests(_ context.Context, req *devopsv1.DiscoverTestsRequest) (*devopsv1.DiscoverTestsResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
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
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	targets := append([]string(nil), req.GetTargets()...)
	if len(targets) == 0 {
		targets = []string{"test"}
	}
	results := make([]*devopsv1.TestResult, 0, len(targets))
	overall := &devopsv1.TaskResult{Status: taskStatusPassed, Summary: "all selected test targets passed"}
	project := detectProject(root)
	resolvedTasks := make([]*devopsv1.BuildTask, 0, len(targets))
	for _, target := range targets {
		task, ok := taskByName(project, target)
		if !ok {
			return nil, serviceerrors.InvalidArgumentf("unknown test target id %q; use an id returned by test_DiscoverTests or omit targets to run the default; available ids: %s", target, strings.Join(testTargetIDs(project), ", "))
		}
		resolvedTasks = append(resolvedTasks, task)
	}
	for i, target := range targets {
		task := resolvedTasks[i]
		if req.GetDryRun() {
			results = append(results, &devopsv1.TestResult{Id: target, Status: taskStatusPlanned, Summary: task.GetCommand()})
			continue
		}
		result, err := runResolvedCommand(ctx, s.commands, root, task.GetCommand())
		if err != nil {
			return nil, operationError(err)
		}
		testResult := &devopsv1.TestResult{Id: target, Status: result.GetStatus(), Summary: result.GetSummary()}
		if result.GetStatus() != taskStatusPassed {
			testResult.Evidence = boundedEvidence(result.GetLogs())
			overall = &devopsv1.TaskResult{
				Status:   result.GetStatus(),
				ExitCode: result.GetExitCode(),
				Summary:  result.GetSummary(),
			}
		}
		results = append(results, testResult)
	}
	if req.GetDryRun() {
		overall = &devopsv1.TaskResult{Status: taskStatusPlanned, Summary: "test targets planned"}
	}
	return &devopsv1.RunTestsResponse{Result: overall, Tests: results}, nil
}

func testTargetIDs(project *devopsv1.Project) []string {
	ids := make([]string, 0)
	for _, task := range project.GetTasks() {
		if strings.Contains(task.GetName(), "test") {
			ids = append(ids, task.GetName())
		}
	}
	return ids
}

func (s *Server) ExplainFailure(_ context.Context, req *devopsv1.ExplainFailureRequest) (*devopsv1.ExplainFailureResponse, error) {
	lines := boundedEvidence(req.GetEvidence())
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
