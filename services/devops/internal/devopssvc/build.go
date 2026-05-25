package devopssvc

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

func (s *Server) DetectProject(_ context.Context, req *devopsv1.DetectProjectRequest) (*devopsv1.DetectProjectResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	return &devopsv1.DetectProjectResponse{Project: detectProject(root)}, nil
}

func (s *Server) ResolveTask(_ context.Context, req *devopsv1.ResolveTaskRequest) (*devopsv1.ResolveTaskResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	taskName := strings.TrimSpace(req.GetTask())
	if task, ok := taskByName(detectProject(root), taskName); ok {
		return &devopsv1.ResolveTaskResponse{Task: task}, nil
	}
	return nil, serviceerrors.NotFoundf("task %q not found", taskName)
}

func (s *Server) RunTask(ctx context.Context, req *devopsv1.RunTaskRequest) (*devopsv1.RunTaskResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	task, ok := taskByName(detectProject(root), strings.TrimSpace(req.GetTask()))
	if !ok {
		return nil, serviceerrors.NotFoundf("task %q not found", strings.TrimSpace(req.GetTask()))
	}
	plan := mutationPlan("build.run_task", task.GetName(), req.GetReason(), true, "command.execute")
	if req.GetDryRun() {
		return &devopsv1.RunTaskResponse{Plan: plan, Result: plannedResult(task.GetCommand())}, nil
	}
	result, err := runResolvedCommand(ctx, s.commands, root, task.GetCommand())
	if err != nil {
		return nil, operationError(err)
	}
	return &devopsv1.RunTaskResponse{Plan: plan, Result: result}, nil
}

func (s *Server) CreateArtifact(_ context.Context, req *devopsv1.CreateArtifactRequest) (*devopsv1.CreateArtifactResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	task := strings.TrimSpace(req.GetTask())
	if task == "" {
		task = "build"
	}
	outputDir, err := s.workspaces.ContainedPath(root, req.GetOutputDir(), "dist")
	if err != nil {
		return nil, operationError(err)
	}
	artifact := &devopsv1.Artifact{Name: task, Kind: "build-output", Uri: outputDir}
	return &devopsv1.CreateArtifactResponse{
		Plan:      mutationPlan("build.create_artifact", outputDir, req.GetReason(), true, "artifact.write"),
		Artifacts: []*devopsv1.Artifact{artifact},
	}, nil
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
		project.Tasks = append(project.Tasks, &devopsv1.BuildTask{Name: "docker-build-plan", Command: "docker build .", Inputs: []string{"Dockerfile"}, Outputs: []string{"container-image"}})
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

func plannedResult(command string) *devopsv1.TaskResult {
	return &devopsv1.TaskResult{Status: taskStatusPlanned, Summary: command}
}
