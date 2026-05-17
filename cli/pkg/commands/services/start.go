package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a supervisor-managed service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
			}
			resp, err := newSupervisorClient().StartService(cmd.Context(), space, args[0])
			if err != nil {
				return serviceCommandError("service start", err)
			}
			if resp.Message != "" {
				fmt.Println(resp.Message)
			}
			fmt.Print(formatServiceInspect(resp.Service))
			return nil
		},
	}
}
