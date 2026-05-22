// Package configcmd provides CLI commands for managing per-space agent
// configuration. Config values are stored in the supervisor's KB under
// namespace "config".
package configcmd

import (
	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
)

const configNamespace = "config"

func connectControl(cmd *cobra.Command) (*natsclient.Client, error) {
	return natsclient.ConnectFromEnv(cmd.Context())
}

func NewConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage agent configuration values",
	}
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigListCmd())
	cmd.AddCommand(newConfigDeleteCmd())
	return cmd
}
