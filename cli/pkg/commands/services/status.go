package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show service readiness status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
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
