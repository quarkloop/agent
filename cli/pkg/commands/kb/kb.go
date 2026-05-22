// Package kbcmd provides CLI commands for the per-space knowledge base.
package kbcmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
)

func connectControl(cmd *cobra.Command) (*natsclient.Client, error) {
	return natsclient.ConnectFromEnv(cmd.Context())
}

func NewKBCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kb",
		Short: "Manage the agent knowledge base",
	}
	cmd.AddCommand(newKBGetCmd())
	cmd.AddCommand(newKBSetCmd())
	cmd.AddCommand(newKBDeleteCmd())
	cmd.AddCommand(newKBListCmd())
	return cmd
}

// parseKey splits "namespace/key" into its parts.
func parseKey(s string) (string, string, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid namespace/key %q — must be <namespace>/<key>", s)
	}
	return parts[0], parts[1], nil
}
