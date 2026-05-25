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
			space, err := currentSpaceName(cmd)
			if err != nil {
				return err
			}
			control, err := newControlClient(cmd.Context())
			if err != nil {
				return err
			}
			defer control.Close()
			services, err := control.ListServices(cmd.Context(), space)
			if err != nil {
				return serviceCommandError("list services", err)
			}
			fmt.Print(formatServiceTable(services))
			return nil
		},
	}
}
