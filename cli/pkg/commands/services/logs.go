package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <name>",
		Short: "Show service logs when supervisor manages them",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
			}
			resp, err := newSupervisorClient().ServiceLogs(cmd.Context(), space, args[0])
			if err != nil {
				return serviceCommandError("service logs", err)
			}
			fmt.Println(resp.Message)
			return nil
		},
	}
}
