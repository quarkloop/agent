package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/quarkloop/supervisor/pkg/natshub"
)

func TestRuntimeLifecycleRoutesAreRemoved(t *testing.T) {
	srv := lifecycleRouteServer(t)

	resp, err := srv.app.Test(newRequest(t, http.MethodPost, "/v1/agents", `{"space":"test"}`))
	if err != nil {
		t.Fatalf("post runtime: %v", err)
	}
	if resp.StatusCode < http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	resp, err = srv.app.Test(newRequest(t, http.MethodPost, "/v1/agents/rt-1/stop", ""))
	if err != nil {
		t.Fatalf("stop runtime: %v", err)
	}
	if resp.StatusCode < http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestServiceLifecycleRoutesAreRemoved(t *testing.T) {
	srv := lifecycleRouteServer(t)
	for _, path := range []string{
		"/v1/spaces/test/services/io/logs",
		"/v1/spaces/test/services/io/start",
		"/v1/spaces/test/services/io/stop",
		"/v1/spaces/test/services/io/restart",
	} {
		method := http.MethodPost
		if strings.HasSuffix(path, "/logs") {
			method = http.MethodGet
		}
		resp, err := srv.app.Test(newRequest(t, method, path, ""))
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		if resp.StatusCode < http.StatusBadRequest {
			t.Fatalf("%s %s status = %d", method, path, resp.StatusCode)
		}
	}
}

func lifecycleRouteServer(t *testing.T) *Server {
	t.Helper()
	srv, err := New(Config{
		SpacesDir: t.TempDir(),
		NATS: natshub.Config{
			Mode:        natshub.ModeExternal,
			ExternalURL: "nats://127.0.0.1:4222",
			Accounts:    natshub.DefaultAccounts(),
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	t.Cleanup(func() {
		if srv.spaceConn != nil {
			_ = srv.spaceConn.Close()
		}
		if srv.spaceServiceGRPC != nil {
			srv.spaceServiceGRPC.Stop()
		}
	})
	return srv
}

func newRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}
