package ioshell

import (
	"errors"
	"os/exec"
	"strings"
)

var ErrNotApproved = errors.New("shell execution requires explicit user approval")

func Execute(command string, approved bool) (output string, exitCode int32, err error) {
	if !approved {
		return "", 0, ErrNotApproved
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return "", 0, errors.New("command is required")
	}
	out, runErr := exec.Command("bash", "-c", command).CombinedOutput()
	exitCode = 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = int32(exitErr.ExitCode())
		} else {
			return "", 0, runErr
		}
	}
	return string(out), exitCode, nil
}
