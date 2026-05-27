package devopssvc

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
)

type osCommands struct{}

func (osCommands) Git(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

func (osCommands) GitPatchCheck(ctx context.Context, root, patch string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "apply", "--check")
	cmd.Stdin = strings.NewReader(patch)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("patch check failed: %w\n%s", err, string(output))
}

func (osCommands) Run(ctx context.Context, root, name string, args ...string) (*devopsv1.TaskResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if root != "" {
		cmd.Dir = root
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	logs := nonEmptyLines(string(out))
	if err != nil {
		exitCode := int32(1)
		if cmd.ProcessState != nil {
			exitCode = int32(cmd.ProcessState.ExitCode())
		}
		if len(logs) == 0 {
			logs = []string{err.Error()}
		}
		return &devopsv1.TaskResult{Status: taskStatusFailed, ExitCode: exitCode, Summary: fmt.Sprintf("%s failed", name), Logs: logs}, nil
	}
	return &devopsv1.TaskResult{Status: taskStatusPassed, Summary: fmt.Sprintf("%s completed", name), Logs: logs}, nil
}

func runResolvedCommand(ctx context.Context, commands commandRunner, root, command string) (*devopsv1.TaskResult, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return &devopsv1.TaskResult{Status: taskStatusFailed, ExitCode: 1, Summary: "empty command"}, nil
	}
	return commands.Run(ctx, root, parts[0], parts[1:]...)
}
