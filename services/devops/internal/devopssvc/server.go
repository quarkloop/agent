package devopssvc

import (
	"context"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/services/devops/internal/buildrelease"
)

const (
	taskStatusPlanned = "planned"
	taskStatusPassed  = "passed"
	taskStatusFailed  = "failed"
)

type workspaceResolver interface {
	Root(string) (string, error)
	ContainedPath(string, string, string) (string, error)
	RelativeFiles([]string) ([]string, error)
}

type commandRunner interface {
	Git(context.Context, string, ...string) (string, error)
	GitPatchCheck(context.Context, string, string) error
	Run(context.Context, string, string, ...string) (*devopsv1.TaskResult, error)
}

type releaseRunner interface {
	Init(context.Context, buildrelease.InitRequest) (*buildrelease.InitResult, error)
	DryRun(context.Context, buildrelease.DryRunRequest) (*buildrelease.DryRunResult, error)
	Release(context.Context, buildrelease.ReleaseRequest) (*buildrelease.ReleaseResult, error)
}

type Server struct {
	workspaces workspaceResolver
	commands   commandRunner
	releases   releaseRunner
}

func NewServer() *Server {
	return newServer(localWorkspace{}, osCommands{}, buildrelease.NewRunner())
}

func newServer(workspaces workspaceResolver, commands commandRunner, releases releaseRunner) *Server {
	return &Server{workspaces: workspaces, commands: commands, releases: releases}
}
