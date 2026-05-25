package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/server"
)

var natsMode string
var natsExternalURL string
var natsStateDir string
var natsClientPort int
var natsWebSocketPort int
var natsMonitorPort int
var natsAuditRetention time.Duration
var natsAuditMaxMessages int64
var bundledPluginsDir string
var installedPluginsDir string

// StartCmd creates the "supervisor start" command.
func StartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the supervisor control plane",
		Long: `Start the NATS-native supervisor control plane.

Example:
  supervisor start --nats-client-port 4222`,
		Args: cobra.NoArgs,
		RunE: runStart,
	}

	cmd.Flags().StringVar(&natsMode, "nats-mode", string(natshub.ModeEmbedded), "NATS mode: embedded or external")
	cmd.Flags().StringVar(&natsExternalURL, "nats-url", "", "External NATS URL when --nats-mode=external")
	cmd.Flags().StringVar(&natsStateDir, "nats-state-dir", "", "Supervisor-owned embedded NATS state directory")
	cmd.Flags().IntVar(&natsClientPort, "nats-client-port", 4222, "Embedded NATS client listen port")
	cmd.Flags().IntVar(&natsWebSocketPort, "nats-websocket-port", 9222, "Embedded NATS WebSocket listen port")
	cmd.Flags().IntVar(&natsMonitorPort, "nats-monitor-port", 8222, "Embedded NATS HTTP monitoring listen port")
	cmd.Flags().DurationVar(&natsAuditRetention, "nats-audit-retention", 0, "Retain redacted service-call audit records for this duration; 0 uses the supervisor default")
	cmd.Flags().Int64Var(&natsAuditMaxMessages, "nats-audit-max-messages", 0, "Maximum retained audit records; 0 uses the supervisor default")
	cmd.Flags().StringVar(&bundledPluginsDir, "bundled-plugins-dir", "plugins", "Read-only product plugin bundle root")
	cmd.Flags().StringVar(&installedPluginsDir, "installed-plugins-dir", "", "Supervisor-owned directory for optional installed plugins")

	return cmd
}

func runStart(cmd *cobra.Command, args []string) error {
	natsCfg, err := startNATSConfig()
	if err != nil {
		return err
	}
	cfg := server.Config{
		NATS:                natsCfg,
		BundledPluginsDir:   bundledPluginsDir,
		InstalledPluginsDir: installedPluginsDir,
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
		if natsAuditRetention > 0 {
			cfg.JetStream.AuditRetention = natsAuditRetention
		}
		if natsAuditMaxMessages > 0 {
			cfg.JetStream.AuditMaxMessages = natsAuditMaxMessages
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
