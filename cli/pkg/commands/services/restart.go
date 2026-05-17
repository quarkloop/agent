package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a service when supervisor manages it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
			}
			resp, err := newSupervisorClient().RestartService(cmd.Context(), space, args[0])
			if err != nil {
				return serviceCommandError("service restart", err)
			}
			if resp.Message != "" {
				fmt.Println(resp.Message)
			}
			fmt.Print(formatServiceInspect(resp.Service))
			return nil
		},
	}
}
