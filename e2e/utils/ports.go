//go:build e2e

package utils

import (
	"net"
	"testing"
)

// ReservePort allocates a loopback host port for one isolated Compose binding.
func ReservePort(t testing.TB) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
