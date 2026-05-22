package natshub

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

type Endpoints struct {
	Mode          Mode
	ClientURL     string
	WebSocketURL  string
	MonitoringURL string
	JetStreamDir  string
}

type Hub struct {
	cfg Config

	mu      sync.Mutex
	server  *natsserver.Server
	started bool
}

func New(cfg Config) (*Hub, error) {
	normalized, err := Normalize(cfg)
	if err != nil {
		return nil, err
	}
	return &Hub{cfg: normalized}, nil
}

func (h *Hub) Config() Config {
	h.mu.Lock()
	defer h.mu.Unlock()
	return cloneConfig(h.cfg)
}

func (h *Hub) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return errors.New("nats hub is already started")
	}
	if h.cfg.Mode == ModeExternal {
		h.started = true
		return nil
	}
	if err := os.MkdirAll(h.cfg.StateDir, 0o755); err != nil {
		return fmt.Errorf("create nats state dir: %w", err)
	}
	if h.cfg.JetStream.Enabled {
		if err := os.MkdirAll(h.cfg.JetStream.StoreDir, 0o755); err != nil {
			return fmt.Errorf("create nats jetstream dir: %w", err)
		}
	}
	opts, err := BuildOptions(h.cfg)
	if err != nil {
		return err
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		return fmt.Errorf("create embedded nats server: %w", err)
	}
	srv.Start()
	if !readyForConnections(ctx, srv, h.cfg.ReadyTimeout) {
		srv.Shutdown()
		srv.WaitForShutdown()
		return errors.New("embedded nats server did not become ready")
	}
	h.server = srv
	h.started = true
	return nil
}

func (h *Hub) Stop(ctx context.Context) error {
	h.mu.Lock()
	srv := h.server
	h.server = nil
	h.started = false
	h.mu.Unlock()

	if srv == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		srv.Shutdown()
		srv.WaitForShutdown()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (h *Hub) Endpoints() Endpoints {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg.Mode == ModeExternal {
		return Endpoints{
			Mode:      h.cfg.Mode,
			ClientURL: h.cfg.ExternalURL,
		}
	}
	out := Endpoints{
		Mode:         h.cfg.Mode,
		JetStreamDir: h.cfg.JetStream.StoreDir,
	}
	if h.server != nil {
		out.ClientURL = h.server.ClientURL()
		out.WebSocketURL = h.server.WebsocketURL()
		out.MonitoringURL = monitoringURL(h.server.MonitorAddr())
		return out
	}
	out.ClientURL = listenerURL("nats", h.cfg.Client.Host, h.cfg.Client.Port)
	if h.cfg.WebSocket.Enabled {
		out.WebSocketURL = listenerURL("ws", h.cfg.WebSocket.Host, h.cfg.WebSocket.Port)
	}
	if h.cfg.Monitoring.Enabled {
		out.MonitoringURL = listenerURL("http", h.cfg.Monitoring.Host, h.cfg.Monitoring.Port)
	}
	return out
}

func (h *Hub) ServerForTest() *natsserver.Server {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.server
}

func readyForConnections(ctx context.Context, srv *natsserver.Server, timeoutDuration time.Duration) bool {
	done := make(chan bool, 1)
	go func() {
		done <- srv.ReadyForConnections(timeoutDuration)
	}()
	select {
	case <-ctx.Done():
		return false
	case ok := <-done:
		return ok
	}
}

func monitoringURL(addr *net.TCPAddr) string {
	if addr == nil {
		return ""
	}
	host := addr.IP.String()
	if host == "" || host == "<nil>" {
		host = defaultMonitoringHost
	}
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, fmt.Sprint(addr.Port))}).String()
}

func listenerURL(scheme, host string, port int) string {
	if host == "" {
		host = "127.0.0.1"
	}
	return (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, fmt.Sprint(port))}).String()
}
