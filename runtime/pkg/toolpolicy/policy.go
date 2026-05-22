package toolpolicy

import (
	"fmt"
	"strings"

	"github.com/quarkloop/pkg/boundary"
)

type Invocation struct {
	Name            string
	Arguments       string
	RuntimeApproved bool
}

var ioFunctionsRequiringApproval = map[string]struct{}{
	"io_Write":    {},
	"io_Append":   {},
	"io_Replace":  {},
	"io_Remove":   {},
	"io_Execute":  {},
}

func Validate(inv Invocation) error {
	name := strings.TrimSpace(inv.Name)
	if _, ok := ioFunctionsRequiringApproval[name]; !ok {
		return nil
	}
	if inv.RuntimeApproved {
		return nil
	}
	return boundary.New(
		boundary.Runtime,
		boundary.ApprovalRequired,
		"tool."+name,
		fmt.Sprintf("service function %q requires explicit runtime approval before execution", name),
	)
}
