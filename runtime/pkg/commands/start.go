package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/quarkloop/runtime/pkg/runtime"
	"github.com/quarkloop/runtime/pkg/startup"
)

const CmdStartDefaultPort = 8765

// Start creates the "runtime start" command.
func Start() *cobra.Command {
	var channelsFlag []string

	cmd := &cobra.Command{
		Use:           "start [channels...]",
		Short:         "start the runtime",
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			channels := startChannels(cmd, args, channelsFlag)
			if len(channels) == 0 {
				channels = []string{"nats"}
			}
			return runStart(channels)
		},
	}

	cmd.Flags().StringSliceVarP(&channelsFlag, "channel", "c", []string{"nats"}, "Channels to use")

	return cmd
}

func startChannels(cmd *cobra.Command, args []string, flagValues []string) []string {
	var channels []string
	if cmd.Flags().Changed("channel") || len(args) == 0 {
		channels = append(channels, flagValues...)
	}
	for _, arg := range args {
		if arg != "channel" && arg != "channels" {
			channels = append(channels, arg)
		}
	}
	return channels
}

func runStart(channels []string) error {
	if os.Getenv(startup.EnvSupervisorURL) == "" {
		loadEnvFiles()
	}

	validChannels, err := startup.ResolveChannels(channels)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := startup.EnvironmentFromOS()
	spaces := env.Spaces()
	if err := startup.EnsurePrimarySpaceEnv(spaces); err != nil {
		return fmt.Errorf("set primary runtime space: %w", err)
	}
	leaseManager, leases, err := startup.ClaimRuntimeSpaces(ctx, spaces)
	if err != nil {
		return err
	}
	defer startup.ReleaseRuntimeSpaces(context.Background(), leases, leaseManager)

	slog.Info("starting runtime")
	slog.Info("enabled channels", "channels", fmt.Sprintf("%v", validChannels))

	srv := runtime.NewServer()
	registrar := startup.AgentRegistrar{Environment: env}
	if err := registrar.Register(ctx, srv, spaces); err != nil {
		return err
	}

	slog.Info("runtime server is running, press Ctrl+C to exit")
	return srv.Run(ctx)
}
