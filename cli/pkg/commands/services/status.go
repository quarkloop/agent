package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [name]",
		Short: "Show service readiness status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				service, err := newSupervisorClient().InspectService(cmd.Context(), space, args[0])
				if err != nil {
					return serviceCommandError("service status", err)
				}
				fmt.Print(formatServiceInspect(service))
				return nil
			}
			services, err := newSupervisorClient().ListServices(cmd.Context(), space)
			if err != nil {
				return serviceCommandError("service status", err)
			}
			fmt.Print(formatServiceTable(services))
			return nil
		},
	}
}
