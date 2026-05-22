package kbcmd

import (
	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
)

func newKBGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <namespace/key>",
		Short: "Read a KB entry",
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
			val, err := control.KBGet(cmd.Context(), name, ns, key)
			if err != nil {
				return err
			}
			cmd.Print(string(val))
			return nil
		},
	}
}
