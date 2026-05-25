// Package doctorcmd provides the `quark doctor` command.
package doctorcmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
	"github.com/quarkloop/cli/pkg/spacecontext"
)

// NewDoctorCommand returns the "doctor" command.
func NewDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run supervisor-side health checks against the current space",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := spacecontext.FromCommand(cmd)
			if err != nil {
				return err
			}
			control, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer control.Close()
			resp, err := control.Doctor(cmd.Context(), name)
			if err != nil {
				return err
			}
			if resp.OK {
				fmt.Println("All checks passed.")
				return nil
			}
			for _, issue := range resp.Issues {
				fmt.Printf("[%s] %s\n", issue.Severity, issue.Message)
			}
			return fmt.Errorf("doctor reported %d issue(s)", len(resp.Issues))
		},
	}
}
