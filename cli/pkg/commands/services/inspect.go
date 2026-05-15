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
			service, err := newSupervisorClient().InspectService(cmd.Context(), space, args[0])
			if err != nil {
				return serviceCommandError("inspect service", err)
			}
			fmt.Print(formatServiceInspect(service))
			return nil
		},
	}
}
