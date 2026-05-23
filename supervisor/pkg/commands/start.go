package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/server"
)

var port int
var natsMode string
var natsExternalURL string
var natsStateDir string
var natsClientPort int
var natsWebSocketPort int
var natsMonitorPort int
var natsArtifactHandoffMaxBytes int64

// StartCmd creates the "supervisor start" command.
func StartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start [spaces-dir]",
		Short: "Start the supervisor server",
		Long: `Start the supervisor HTTP server that manages Spaces.

Example:
  supervisor start --port 7200`,
		RunE: runStart,
	}

	cmd.Flags().IntVarP(&port, "port", "p", 7200, "HTTP listen port")
	cmd.Flags().StringVar(&natsMode, "nats-mode", string(natshub.ModeEmbedded), "NATS mode: embedded or external")
	cmd.Flags().StringVar(&natsExternalURL, "nats-url", "", "External NATS URL when --nats-mode=external")
	cmd.Flags().StringVar(&natsStateDir, "nats-state-dir", "", "Supervisor-owned embedded NATS state directory")
	cmd.Flags().IntVar(&natsClientPort, "nats-client-port", 4222, "Embedded NATS client listen port")
	cmd.Flags().IntVar(&natsWebSocketPort, "nats-websocket-port", 9222, "Embedded NATS WebSocket listen port")
	cmd.Flags().IntVar(&natsMonitorPort, "nats-monitor-port", 8222, "Embedded NATS HTTP monitoring listen port")
	cmd.Flags().Int64Var(&natsArtifactHandoffMaxBytes, "nats-artifact-handoff-max-bytes", 0, "Embedded NATS artifact handoff object-store max bytes; 0 uses the supervisor default")

	return cmd
}

func runStart(cmd *cobra.Command, args []string) error {
	// When no positional arg is given, leave SpacesDir empty so
	// server.New falls back to space.DefaultRoot (which honours
	// QUARK_SPACES_ROOT or $HOME/.quarkloop/spaces).
	var spacesDir string
	if len(args) > 0 {
		spacesDir = args[0]
	}

	natsCfg, err := startNATSConfig()
	if err != nil {
		return err
	}
	cfg := server.Config{
		Port:      port,
		SpacesDir: spacesDir,
		NATS:      natsCfg,
	}
	srv, err := server.New(cfg)
	if err != nil {
		return err
	}

	return srv.Start(context.Background())
}

func startNATSConfig() (natshub.Config, error) {
	switch natshub.Mode(strings.TrimSpace(natsMode)) {
	case natshub.ModeEmbedded, "":
		stateDir := strings.TrimSpace(natsStateDir)
		if stateDir == "" {
			defaultDir, err := natshub.DefaultStateDir()
			if err != nil {
				return natshub.Config{}, err
			}
			stateDir = defaultDir
		}
		cfg := natshub.DefaultConfig(stateDir)
		cfg.Client.Port = natsClientPort
		cfg.WebSocket.Port = natsWebSocketPort
		cfg.Monitoring.Port = natsMonitorPort
		if natsArtifactHandoffMaxBytes > 0 {
			cfg.JetStream.ArtifactHandoffMaxBytes = natsArtifactHandoffMaxBytes
		}
		return cfg, nil
	case natshub.ModeExternal:
		return natshub.Config{
			Mode:        natshub.ModeExternal,
			ExternalURL: strings.TrimSpace(natsExternalURL),
			Accounts:    natshub.DefaultAccounts(),
		}, nil
	default:
		return natshub.Config{}, fmt.Errorf("unsupported nats mode %q", natsMode)
	}
}
