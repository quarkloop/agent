package plugincmd

import (
	"fmt"

	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
)

func newSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search the plugin hub",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := spacemodel.CurrentName()
			if err != nil {
				return err
			}
			control, err := connectControl(cmd)
			if err != nil {
				return err
			}
			defer control.Close()
			results, err := control.SearchPlugins(cmd.Context(), name, args[0])
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			if len(results) == 0 {
				fmt.Println("No plugins found.")
				return nil
			}
			for _, r := range results {
				fmt.Printf("%-24s %-10s %s\n", r.Name, r.Version, r.Description)
			}
			return nil
		},
	}
}
