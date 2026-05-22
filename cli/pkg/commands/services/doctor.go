package servicescmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run service diagnostics through the supervisor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			space, err := currentSpaceName()
			if err != nil {
				return err
			}
			control, err := newControlClient(cmd.Context())
			if err != nil {
				return err
			}
			defer control.Close()
			resp, err := control.ServiceDoctor(cmd.Context(), space)
			if err != nil {
				return serviceCommandError("service doctor", err)
			}
			fmt.Print(formatServiceDoctor(resp))
			return nil
		},
	}
}
