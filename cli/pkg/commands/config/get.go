package configcmd

import (
	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
)

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Read a configuration value",
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
			val, err := control.KBGet(cmd.Context(), name, configNamespace, args[0])
			if err != nil {
				return err
			}
			cmd.Print(string(val))
			return nil
		},
	}
}
