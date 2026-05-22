// Package servicescmd provides service-manager commands backed by NATS control subjects.
package servicescmd

import "github.com/spf13/cobra"

func NewServicesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Inspect Quark services through the supervisor",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newInspectCmd())
	cmd.AddCommand(newDoctorCmd())
	return cmd
}
