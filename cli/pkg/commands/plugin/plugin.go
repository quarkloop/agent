// Package plugincmd provides the root command for plugin management.
// All operations are delegated to the supervisor HTTP API.
package plugincmd

import (
	"github.com/spf13/cobra"
)

// NewPluginCommand creates the plugin subcommand tree.
func NewPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage agent plugins (delegated to the supervisor)",
	}

	cmd.AddCommand(newSearchCmd())
	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newUninstallCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newInfoCmd())

	return cmd
}
