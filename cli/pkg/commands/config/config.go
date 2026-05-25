// Package configcmd provides CLI access to the authoritative space config.
package configcmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
	"github.com/quarkloop/cli/pkg/spacecontext"
	spacemodel "github.com/quarkloop/pkg/space"
)

func connectControl(cmd *cobra.Command) (*natsclient.Client, error) {
	return natsclient.ConnectFromEnv(cmd.Context())
}

func NewConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the authoritative space configuration",
	}
	cmd.AddCommand(newShowCommand())
	cmd.AddCommand(newApplyCommand())
	return cmd
}

func newShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the authoritative space.json configuration",
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
			config, err := control.SpaceConfig(cmd.Context(), name)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(config.Config)
			return err
		},
	}
}

func newApplyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <path>",
		Short: "Replace space configuration from a validated JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := spacecontext.FromCommand(cmd)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}
			if _, err := spacemodel.ParseAndValidateConfig(data, name); err != nil {
				return fmt.Errorf("validate config: %w", err)
			}
			control, err := connectControl(cmd)
			if err != nil {
				return err
			}
			defer control.Close()
			_, err = control.UpdateSpace(cmd.Context(), name, data)
			return err
		},
	}
}
