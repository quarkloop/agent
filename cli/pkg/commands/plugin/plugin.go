// Package plugincmd provides the root command for plugin management.
package plugincmd

import (
	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
)

func connectControl(cmd *cobra.Command) (*natsclient.Client, error) {
	return natsclient.ConnectFromEnv(cmd.Context())
}

// NewPluginCommand creates the plugin subcommand tree.
func NewPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage agent plugins",
	}

	cmd.AddCommand(newSearchCmd())
	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newUninstallCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newInfoCmd())

	return cmd
}
