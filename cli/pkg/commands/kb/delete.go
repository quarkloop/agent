package kbcmd

import (
	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
)

func newKBDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <namespace/key>",
		Short: "Delete a KB entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ns, key, err := parseKey(args[0])
			if err != nil {
				return err
			}
			name, err := spacemodel.CurrentName()
			if err != nil {
				return err
			}
			control, err := connectControl(cmd)
			if err != nil {
				return err
			}
			defer control.Close()
			return control.KBDelete(cmd.Context(), name, ns, key)
		},
	}
}
