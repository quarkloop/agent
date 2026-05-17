package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a supervisor-managed service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
			}
			resp, err := newSupervisorClient().StopService(cmd.Context(), space, args[0])
			if err != nil {
				return serviceCommandError("service stop", err)
			}
			if resp.Message != "" {
				fmt.Println(resp.Message)
			}
			fmt.Print(formatServiceInspect(resp.Service))
			return nil
		},
	}
}
