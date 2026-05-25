package plugincmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/spacecontext"
)

func newListCmd() *cobra.Command {
	var typeFilter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := spacecontext.FromCommand(cmd)
			if err != nil {
				return err
			}
			control, err := connectControl(cmd)
			if err != nil {
				return err
			}
			defer control.Close()
			plugins, err := control.ListPlugins(cmd.Context(), name, typeFilter)
			if err != nil {
				return fmt.Errorf("list failed: %w", err)
			}
			if len(plugins) == 0 {
				fmt.Println("No plugins installed.")
				return nil
			}
			for _, p := range plugins {
				fmt.Printf("%-24s %-10s %-10s %s\n", p.Name, p.Version, p.Type, p.Description)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by plugin type (tool, agent, skill, service)")
	return cmd
}
