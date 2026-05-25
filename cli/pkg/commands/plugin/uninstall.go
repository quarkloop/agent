package plugincmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/spacecontext"
)

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Uninstall a plugin from the current space",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := spacecontext.FromCommand(cmd)
			if err != nil {
				return err
			}
			control, err := connectControl(cmd)
			if err != nil {
				return err
			}
			defer control.Close()
			if err := control.UninstallPlugin(cmd.Context(), name, args[0]); err != nil {
				return fmt.Errorf("uninstall failed: %w", err)
			}
			fmt.Printf("Uninstalled %s\n", args[0])
			return nil
		},
	}
}
