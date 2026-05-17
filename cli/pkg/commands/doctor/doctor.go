// Package doctorcmd provides the `quark doctor` command.
package doctorcmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

// NewDoctorCommand returns the "doctor" command.
func NewDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run supervisor-side health checks against the current space",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			name, err := spacemodel.NameFromDir(cwd)
			if err != nil {
				return err
			}
			resp, err := supclient.New().Doctor(cmd.Context(), name)
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
