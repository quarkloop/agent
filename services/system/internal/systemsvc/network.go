package systemsvc

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
)

type procNetwork struct{}

func (procNetwork) Ports(protocol string, limit int) ([]*systemv1.Port, error) {
	connections, err := (procNetwork{}).Connections("", limit*2)
	if err != nil {
		return nil, err
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	ports := make([]*systemv1.Port, 0)
	seen := map[string]struct{}{}
	for _, conn := range connections {
		if conn.GetState() != "LISTEN" && conn.GetProtocol() != "udp" && conn.GetProtocol() != "udp6" {
			continue
		}
		if protocol != "" && !strings.HasPrefix(conn.GetProtocol(), protocol) {
			continue
		}
		key := conn.GetProtocol() + "|" + conn.GetLocalAddress() + "|" + fmt.Sprint(conn.GetLocalPort())
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ports = append(ports, &systemv1.Port{Protocol: conn.GetProtocol(), Address: conn.GetLocalAddress(), Port: conn.GetLocalPort(), Pid: conn.GetPid()})
		if len(ports) >= limit {
			break
		}
	}
	return ports, nil
}

func (procNetwork) Connections(state string, limit int) ([]*systemv1.NetworkConnection, error) {
	files := []struct{ path, protocol string }{
		{"/proc/net/tcp", "tcp"}, {"/proc/net/tcp6", "tcp6"},
		{"/proc/net/udp", "udp"}, {"/proc/net/udp6", "udp6"},
	}
	state = strings.ToUpper(strings.TrimSpace(state))
	out := make([]*systemv1.NetworkConnection, 0)
	for _, file := range files {
		connections, err := parseNetFile(file.path, file.protocol)
		if err != nil {
			continue
		}
		for _, conn := range connections {
			if state == "" || conn.GetState() == state {
				out = append(out, conn)
				if len(out) >= limit {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

func parseNetFile(path, protocol string) ([]*systemv1.NetworkConnection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make([]*systemv1.NetworkConnection, 0)
	for i, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if i == 0 || len(fields) < 4 {
			continue
		}
		localAddr, localPort := parseEndpoint(fields[1])
		remoteAddr, remotePort := parseEndpoint(fields[2])
		out = append(out, &systemv1.NetworkConnection{
			Protocol: protocol, LocalAddress: localAddr, LocalPort: localPort,
			RemoteAddress: remoteAddr, RemotePort: remotePort, State: tcpState(fields[3]),
		})
	}
	return out, nil
}

func parseEndpoint(raw string) (string, uint32) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return "", 0
	}
	port64, _ := strconv.ParseUint(parts[1], 16, 32)
	addr := parts[0]
	if len(addr) == 8 {
		bytes := make([]byte, 4)
		for i := 0; i < 4; i++ {
			v, _ := strconv.ParseUint(addr[i*2:i*2+2], 16, 8)
			bytes[3-i] = byte(v)
		}
		return net.IP(bytes).String(), uint32(port64)
	}
	return addr, uint32(port64)
}

func tcpState(code string) string {
	switch code {
	case "0A":
		return "LISTEN"
	case "01":
		return "ESTABLISHED"
	case "06":
		return "TIME_WAIT"
	case "07":
		return "CLOSE"
	default:
		return code
	}
}
