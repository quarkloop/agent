package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List services resolved by the supervisor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
			}
			services, err := newSupervisorClient().ListServices(cmd.Context(), space)
			if err != nil {
				return serviceCommandError("list services", err)
			}
			fmt.Print(formatServiceTable(services))
			return nil
		},
	}
}
