package plugincmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/spacecontext"
)

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show installed plugin details",
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
			p, err := control.GetPlugin(cmd.Context(), name, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Name:        %s\n", p.Name)
			fmt.Printf("Version:     %s\n", p.Version)
			fmt.Printf("Type:        %s\n", p.Type)
			fmt.Printf("Mode:        %s\n", p.Mode)
			fmt.Printf("Description: %s\n", p.Description)
			return nil
		},
	}
}
