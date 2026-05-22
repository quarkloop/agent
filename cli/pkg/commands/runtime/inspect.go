package runtimecmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

// NewInspectCommand returns the "inspect" command.
func NewInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect",
		Short: "Show status and details of the current space and its runtime",
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
			sup := supclient.New()
			info, err := sup.GetSpace(cmd.Context(), name)
			if err != nil {
				return err
			}
			fmt.Printf("Space:      %s\n", info.Name)
			if info.Version != "" {
				fmt.Printf("Version:    %s\n", info.Version)
			}
			fmt.Printf("Created:    %s\n", info.CreatedAt.Format(time.RFC3339))
			fmt.Printf("Updated:    %s\n", info.UpdatedAt.Format(time.RFC3339))

			fmt.Println("Runtime:    deployment-managed")
			return nil
		},
	}
}
