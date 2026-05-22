package configcmd

import (
	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
)

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Write a configuration value",
		Args:  cobra.ExactArgs(2),
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
			return control.KBSet(cmd.Context(), name, configNamespace, args[0], []byte(args[1]))
		},
	}
}
