// Package server composes the supervisor NATS-native control plane.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/supervisor/internal/supervisor"
	"github.com/quarkloop/supervisor/pkg/events"
	"github.com/quarkloop/supervisor/pkg/natsapi"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/space"
	"github.com/quarkloop/supervisor/pkg/space/remotestore"
)

// Config holds supervisor control-plane configuration.
type Config struct {
	// NATS configures the supervisor-owned NATS hub.
	NATS natshub.Config
	// BundledPluginsDir is the immutable product plugin bundle root. Space
	// configuration selects entries from this catalog by manifest identity.
	BundledPluginsDir string
	// InstalledPluginsDir is supervisor-owned persistence for optional plugin
	// installations. Space config stores selection only.
	InstalledPluginsDir string
	// Store is an injected semantic store for focused tests only. Product
	// startup always establishes the Space-service-backed store after NATS is
	// ready.
	Store space.Store
}

// Server is the supervisor NATS control-plane host.
type Server struct {
	cfg Config

	store       space.Store
	storeCloser interface{ Close() }
	events      *events.Bus
	natsHub     *natshub.Hub
	natsAPI     *natsapi.Server

	pluginRegistry    *pluginmanager.Registry
	pluginSelectionMu sync.Mutex
}

// New creates a new supervisor control plane.
func New(cfg Config) (*Server, error) {
	if cfg.BundledPluginsDir == "" {
		cfg.BundledPluginsDir = "plugins"
	}
	if cfg.NATS.Mode == "" && cfg.NATS.StateDir == "" && cfg.NATS.ExternalURL == "" {
		stateDir, err := natshub.DefaultStateDir()
		if err != nil {
			return nil, fmt.Errorf("resolve nats state dir: %w", err)
		}
		cfg.NATS = natshub.DefaultConfig(stateDir)
	}
	natsHub, err := natshub.New(cfg.NATS)
	if err != nil {
		return nil, fmt.Errorf("configure nats hub: %w", err)
	}
	if cfg.InstalledPluginsDir == "" {
		pluginStateDir, err := defaultInstalledPluginsDir(cfg.NATS)
		if err != nil {
			return nil, err
		}
		cfg.InstalledPluginsDir = pluginStateDir
	}
	registry, err := pluginmanager.NewRegistry(cfg.BundledPluginsDir, cfg.InstalledPluginsDir)
	if err != nil {
		return nil, fmt.Errorf("configure plugin registry: %w", err)
	}
	if _, err := registry.Get("quark-main"); err != nil {
		return nil, fmt.Errorf("required main agent plugin: %w", err)
	}

	s := &Server{
		cfg:            cfg,
		store:          cfg.Store,
		events:         events.NewBus(),
		natsHub:        natsHub,
		pluginRegistry: registry,
	}
	return s, nil
}

func defaultInstalledPluginsDir(cfg natshub.Config) (string, error) {
	if cfg.StateDir != "" {
		return filepath.Join(filepath.Dir(cfg.StateDir), "plugins"), nil
	}
	stateDir, err := natshub.DefaultStateDir()
	if err != nil {
		return "", fmt.Errorf("resolve plugin state dir: %w", err)
	}
	return filepath.Join(filepath.Dir(stateDir), "plugins"), nil
}

// Start starts the NATS-native supervisor control plane and blocks until ctx
// is cancelled.
func (s *Server) Start(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	sup := supervisor.New()
	if s.natsHub != nil {
		if err := s.natsHub.Start(ctx); err != nil {
			return fmt.Errorf("start nats hub: %w", err)
		}
		defer func() {
			shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := s.natsHub.Stop(shutCtx); err != nil {
				slog.Error("failed to stop nats hub", "error", err)
			}
		}()
		endpoints := s.natsHub.Endpoints()
		slog.Info("nats hub ready",
			"mode", endpoints.Mode,
			"client_url", endpoints.ClientURL,
			"websocket_url", endpoints.WebSocketURL,
			"monitoring_url", endpoints.MonitoringURL,
			"jetstream_dir", endpoints.JetStreamDir,
		)
		controlCredential, err := s.natsHub.ControlCredential()
		if err != nil {
			return fmt.Errorf("resolve nats control credential: %w", err)
		}
		if s.store == nil {
			remote, err := remotestore.New(ctx, natskit.Config{
				URL:             endpoints.ClientURL,
				Username:        controlCredential.Username,
				Password:        controlCredential.Password,
				Name:            "quark-supervisor-space-persistence",
				AuditPrefix:     "audit",
				TelemetryPrefix: "telemetry",
			})
			if err != nil {
				return fmt.Errorf("connect space persistence: %w", err)
			}
			s.store = remote
			s.storeCloser = remote
			defer func() {
				s.storeCloser.Close()
				s.storeCloser = nil
			}()
		}
		apiServer, err := natsapi.Start(ctx, natsapi.Config{
			URL:      endpoints.ClientURL,
			Username: controlCredential.Username,
			Password: controlCredential.Password,
		}, s.store, s.events, s.natsHub,
			natsapi.WithServiceInspector(natsServiceInspector{server: s}),
			natsapi.WithCatalogResolver(s),
			natsapi.WithPluginController(s),
		)
		if err != nil {
			return fmt.Errorf("start nats control api: %w", err)
		}
		s.natsAPI = apiServer
		defer func() {
			s.natsAPI.Close()
			s.natsAPI = nil
		}()
	}
	if err := sup.Save(supervisor.State{PID: os.Getpid()}); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	defer sup.Clear()
	<-ctx.Done()
	return nil
}

func Stop() error {
	sup := supervisor.New()

	state, err := sup.Load()
	if err != nil {
		return err // "supervisor is not running"
	}

	return stopSupervisorProcess(state.PID)
}
