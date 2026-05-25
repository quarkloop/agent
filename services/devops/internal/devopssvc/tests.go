package devopssvc

import (
	"context"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
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
	for _, target := range targets {
		task, ok := taskByName(project, target)
		if !ok {
			results = append(results, &devopsv1.TestResult{Id: target, Status: taskStatusFailed, Summary: "test target not found"})
			overall.Status = taskStatusFailed
			continue
		}
		if req.GetDryRun() {
			results = append(results, &devopsv1.TestResult{Id: target, Status: taskStatusPlanned, Summary: task.GetCommand()})
			continue
		}
		result, err := runResolvedCommand(ctx, s.commands, root, task.GetCommand())
		if err != nil {
			return nil, operationError(err)
		}
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
