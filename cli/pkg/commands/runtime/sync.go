package runtimecmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
	spacemodel "github.com/quarkloop/pkg/space"
)

// NewSyncCommand returns the "sync" command.
func NewSyncCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync the local Quarkfile to the supervisor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			data, err := spacemodel.ReadQuarkfile(cwd)
			if err != nil {
				return err
			}
			name, err := spacemodel.NameFromQuarkfile(data)
			if err != nil {
				return err
			}
			if _, err := spacemodel.ParseAndValidateQuarkfileForSpace(data, name); err != nil {
				return err
			}
			control, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer control.Close()
			info, err := control.UpdateSpace(cmd.Context(), name, data)
			if err != nil {
				return fmt.Errorf("sync Quarkfile: %w", err)
			}
			fmt.Printf("Quarkfile synced: %s\n", info.Name)
			return nil
		},
	}
}
