package toolpolicy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quarkloop/pkg/boundary"
)

type Invocation struct {
	Name            string
	Arguments       string
	RuntimeApproved bool
}

func Validate(inv Invocation) error {
	if strings.TrimSpace(inv.Name) != "fs" {
		return nil
	}
	args, ok := decodeArguments(inv.Arguments)
	if !ok {
		return nil
	}
	command := argumentString(args, "command")
	if !fsCommandMutates(command) {
		return nil
	}
	if inv.RuntimeApproved {
		return nil
	}
	return boundary.New(
		boundary.Runtime,
		boundary.ApprovalRequired,
		"tool.fs."+command,
		fmt.Sprintf("filesystem %q requires explicit runtime approval before execution", command),
	)
}

func decodeArguments(arguments string) (map[string]any, bool) {
	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return nil, false
	}
	return args, true
}

func argumentString(args map[string]any, name string) string {
	for _, candidate := range []string{name, strings.ReplaceAll(name, "-", "_"), strings.ReplaceAll(name, "_", "-")} {
		if raw, ok := args[candidate]; ok {
			value, _ := raw.(string)
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fsCommandMutates(command string) bool {
	switch command {
	case "write", "append", "replace", "rm":
		return true
	default:
		return false
	}
}
