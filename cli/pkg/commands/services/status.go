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
			control, err := newControlClient(cmd.Context())
			if err != nil {
				return err
			}
			defer control.Close()
			if len(args) == 1 {
				service, err := control.InspectService(cmd.Context(), space, args[0])
				if err != nil {
					return serviceCommandError("service status", err)
				}
				fmt.Print(formatServiceInspect(service))
				return nil
			}
			services, err := control.ListServices(cmd.Context(), space)
			if err != nil {
				return serviceCommandError("service status", err)
			}
			fmt.Print(formatServiceTable(services))
			return nil
		},
	}
}
