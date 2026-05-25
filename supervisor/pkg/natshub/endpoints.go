package natshub

import (
	"fmt"
	"net"
	"net/url"
)

type Endpoints struct {
	Mode          Mode
	ClientURL     string
	WebSocketURL  string
	MonitoringURL string
	JetStreamDir  string
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
