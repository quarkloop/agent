package runtimecmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	spacemodel "github.com/quarkloop/pkg/space"
	rtclient "github.com/quarkloop/runtime/pkg/client"
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

			rt, err := sup.RuntimeBySpace(cmd.Context(), name)
			if err != nil {
				if supclient.IsNotFound(err) {
					fmt.Println("Runtime:    (not running)")
					return nil
				}
				return err
			}
			fmt.Printf("Runtime ID: %s\n", rt.ID)
			fmt.Printf("Status:     %s\n", rt.Status)
			if rt.PID != 0 {
				fmt.Printf("PID:        %d\n", rt.PID)
			}
			if rt.Port != 0 {
				fmt.Printf("Port:       %d\n", rt.Port)
			}
			if !rt.StartedAt.IsZero() {
				fmt.Printf("Started:    %s\n", rt.StartedAt.Format(time.RFC3339))
			}
			if rt.Uptime != "" {
				fmt.Printf("Uptime:     %s\n", rt.Uptime)
			}
			if rt.Status != supclient.RuntimeRunning {
				return nil
			}
			runtimeInfo, err := rtclient.New(rt.URL()).Info(cmd.Context())
			if err != nil {
				return nil
			}
			fmt.Printf("Provider:   %s\n", runtimeInfo.Provider)
			fmt.Printf("Model:      %s\n", runtimeInfo.Model)
			fmt.Printf("Mode:       %s\n", runtimeInfo.Mode)
			return nil
		},
	}
}
