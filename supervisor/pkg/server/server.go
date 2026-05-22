// Package server implements the Supervisor HTTP API.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/space/pkg/spacesvc"
	"github.com/quarkloop/supervisor/internal/supervisor"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/events"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/space"
	"github.com/quarkloop/supervisor/pkg/space/grpcstore"
	"google.golang.org/grpc"
)

// Config holds supervisor server configuration.
type Config struct {
	// Port is the TCP port to listen on.
	Port int
	// SpacesDir is the root directory for the filesystem-backed space store.
	// When empty, fsstore.DefaultRoot() is used.
	SpacesDir string
	// SpaceServiceAddr is an existing SpaceService gRPC address. When empty,
	// the supervisor starts an embedded local SpaceService and still talks to
	// it through gRPC.
	SpaceServiceAddr string
	// NATS configures the supervisor-owned NATS hub.
	NATS natshub.Config
}

// Server is the Supervisor HTTP API server.
type Server struct {
	cfg Config
	app *fiber.App

	store   space.Store
	events  *events.Bus
	natsHub *natshub.Hub

	spaceConn        *grpcstore.Store
	spaceServiceGRPC *grpc.Server
}

// New creates a new Supervisor server.
func New(cfg Config) (*Server, error) {
	if cfg.Port == 0 {
		cfg.Port = 7200
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
	root := cfg.SpacesDir
	if root == "" {
		r, err := spacesvc.DefaultRoot()
		if err != nil {
			return nil, fmt.Errorf("resolve spaces root: %w", err)
		}
		root = r
	}

	spaceServiceAddr := cfg.SpaceServiceAddr
	var spaceGRPC *grpc.Server
	if spaceServiceAddr == "" {
		addr, srv, err := startEmbeddedSpaceService(root)
		if err != nil {
			return nil, err
		}
		spaceServiceAddr = addr
		spaceGRPC = srv
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := grpcstore.Dial(dialCtx, spaceServiceAddr)
	if err != nil {
		if spaceGRPC != nil {
			spaceGRPC.Stop()
		}
		return nil, fmt.Errorf("dial space service %s: %w", spaceServiceAddr, err)
	}

	s := &Server{
		cfg:              cfg,
		store:            store,
		events:           events.NewBus(),
		natsHub:          natsHub,
		spaceConn:        store,
		spaceServiceGRPC: spaceGRPC,
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
	}
	// Write state before accepting traffic
	if err := sup.Save(supervisor.State{Port: s.cfg.Port, PID: os.Getpid()}); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	defer sup.Clear() // clean up on exit
	defer func() {
		if s.spaceConn != nil {
			_ = s.spaceConn.Close()
		}
		if s.spaceServiceGRPC != nil {
			s.spaceServiceGRPC.GracefulStop()
		}
	}()

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

func startEmbeddedSpaceService(root string) (string, *grpc.Server, error) {
	store, err := spacesvc.NewStoreWithEnvironment(root, os.Environ())
	if err != nil {
		return "", nil, fmt.Errorf("open space service store: %w", err)
	}
	server, err := spacesvc.NewServer(store)
	if err != nil {
		return "", nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen embedded space service: %w", err)
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(slog.Default())))
	spacev1.RegisterSpaceServiceServer(grpcServer, server)
	registry := servicekit.NewRegistry()
	if err := registry.Register(spacesvc.Descriptor(ln.Addr().String(), embeddedSpaceSkill())); err != nil {
		ln.Close()
		return "", nil, err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)
	go func() {
		if err := grpcServer.Serve(ln); err != nil {
			slog.Error("embedded space service stopped", "error", err)
		}
	}()
	return ln.Addr().String(), grpcServer, nil
}

func embeddedSpaceSkill() *servicev1.SkillDescriptor {
	for _, path := range []string{"services/space/SKILL.md", "../services/space/SKILL.md"} {
		skill, err := servicekit.SkillFromFile("service-space", "1.0.0", path)
		if err == nil {
			return skill
		}
	}
	return nil
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
