package configcmd

import (
	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
)

func newConfigDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <key>",
		Short: "Delete a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := spacemodel.CurrentName()
			if err != nil {
				return err
			}
			control, err := connectControl(cmd)
			if err != nil {
				return err
			}
			defer control.Close()
			return control.KBDelete(cmd.Context(), name, configNamespace, args[0])
		},
	}
}
