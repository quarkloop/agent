package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <name>",
		Short: "Inspect one service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
			}
			control, err := newControlClient(cmd.Context())
			if err != nil {
				return err
			}
			defer control.Close()
			service, err := control.InspectService(cmd.Context(), space, args[0])
			if err != nil {
				return serviceCommandError("inspect service", err)
			}
			fmt.Print(formatServiceInspect(service))
			return nil
		},
	}
}
