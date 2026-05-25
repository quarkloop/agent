// Package spacecontext resolves explicit CLI space selection without reading
// product state from a user working directory.
package spacecontext

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const EnvSpace = "QUARK_SPACE"

// FromCommand resolves --space first and then QUARK_SPACE.
func FromCommand(cmd *cobra.Command) (string, error) {
	if cmd != nil {
		value, err := cmd.Root().PersistentFlags().GetString("space")
		if err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
	return FromEnvironment()
}

// FromEnvironment resolves the selected space for non-command clients.
func FromEnvironment() (string, error) {
	if value := strings.TrimSpace(os.Getenv(EnvSpace)); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("space is required; pass --space <name> or set %s", EnvSpace)
}
