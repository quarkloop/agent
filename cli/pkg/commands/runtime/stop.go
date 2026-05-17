package runtimecmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/util"
	spacemodel "github.com/quarkloop/pkg/space"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

// NewStopCommand returns the "stop" command.
func NewStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the runtime for the current space via the supervisor",
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
			rt, err := sup.RuntimeBySpace(cmd.Context(), name)
			if err != nil {
				if supclient.IsNotFound(err) {
					return fmt.Errorf("no runtime running for space %q", name)
				}
				return err
			}
			stopped, err := sup.StopRuntime(cmd.Context(), rt.ID)
			if err != nil {
				return fmt.Errorf("stop runtime: %w", err)
			}
			util.Successf("Runtime stopped (space=%s, id=%s, status=%s)", stopped.Space, stopped.ID, stopped.Status)
			return nil
		},
	}
}
