// Package server implements the Supervisor HTTP API.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/supervisor/internal/supervisor"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/events"
	"github.com/quarkloop/supervisor/pkg/natsapi"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/pluginbootstrap"
	"github.com/quarkloop/supervisor/pkg/space"
	"github.com/quarkloop/supervisor/pkg/space/remotestore"
)

// Config holds supervisor server configuration.
type Config struct {
	// Port is the TCP port to listen on.
	Port int
	// SpacesDir is retained for deployment validation while persistence is
	// delegated to the configured Space service.
	SpacesDir string
	// NATS configures the supervisor-owned NATS hub.
	NATS natshub.Config
	// BundledPluginsDir is the product plugin bundle root. The supervisor
	// installs required default plugins from this directory for new spaces.
	BundledPluginsDir string
	// Store is an injected semantic store for focused tests only. Product
	// startup always establishes the Space-service-backed store after NATS is
	// ready.
	Store space.Store
}

// Server is the Supervisor HTTP API server.
type Server struct {
	cfg Config
	app *fiber.App

	store       space.Store
	storeCloser interface{ Close() }
	events      *events.Bus
	natsHub     *natshub.Hub
	natsAPI     *natsapi.Server

	pluginBootstrap *pluginbootstrap.Installer
}

// New creates a new Supervisor server.
func New(cfg Config) (*Server, error) {
	if cfg.Port == 0 {
		cfg.Port = 7200
	}
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
	bootstrap, err := pluginbootstrap.New(cfg.BundledPluginsDir)
	if err != nil {
		return nil, fmt.Errorf("configure required plugin bootstrap: %w", err)
	}

	s := &Server{
		cfg:             cfg,
		store:           cfg.Store,
		events:          events.NewBus(),
		natsHub:         natsHub,
		pluginBootstrap: bootstrap,
	}

	fiberConfig := fiber.Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		ErrorHandler: s.errorHandler,
	}
	s.app = fiber.New(fiberConfig)
	s.app.Use(recover.New())
	s.app.Use(logger.New(logger.Config{Format: "${time} ${status} - ${latency} ${method} ${path}\n"}))
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	s.routes()
	return s, nil
}

// Run starts the HTTP server and blocks until ctx is cancelled.
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
			natsapi.WithSpaceBootstrapper(s),
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
	// Write state before accepting traffic
	if err := sup.Save(supervisor.State{Port: s.cfg.Port, PID: os.Getpid()}); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	defer sup.Clear() // clean up on exit

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("supervisor listening on :%d\n", s.cfg.Port)
		if err := s.app.Listen(fmt.Sprintf(":%d", s.cfg.Port)); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return s.app.ShutdownWithContext(shutCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) BootstrapSpace(ctx context.Context, spaceID string) error {
	plugins, err := s.store.Plugins(spaceID)
	if err != nil {
		return fmt.Errorf("open space plugin store: %w", err)
	}
	return s.pluginBootstrap.InstallRequired(ctx, plugins)
}

// errorHandler is the Fiber custom error handler.
func (s *Server) errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(api.ErrorResponse{Error: err.Error()})
}

func Stop() error {
	sup := supervisor.New()

	state, err := sup.Load()
	if err != nil {
		return err // "supervisor is not running"
	}

	return stopSupervisorProcess(state.PID, state.Port)
}
