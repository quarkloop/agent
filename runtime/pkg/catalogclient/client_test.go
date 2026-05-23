package catalogclient

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func TestConfigAvailableRequiresScopedRuntimeCredential(t *testing.T) {
	cfg := Config{URL: "nats://127.0.0.1:4222", Username: "u", Password: "p", SpaceID: "docs"}
	if !cfg.Available() {
		t.Fatal("expected complete config to be available")
	}
	cfg.SpaceID = ""
	if cfg.Available() {
		t.Fatal("expected missing space id to be unavailable")
	}
}

func TestFetchRuntimeCatalog(t *testing.T) {
	srv := startCatalogTestServer(t)
	conn, err := nats.Connect(srv.ClientURL(), nats.UserInfo("runtime", "secret"), nats.Timeout(time.Second))
	if err != nil {
		t.Fatalf("connect responder: %v", err)
	}
	t.Cleanup(conn.Close)
	if _, err := conn.Subscribe(clientcontract.SubjectCatalogRuntimeGet, func(msg *nats.Msg) {
		var req clientcontract.RequestEnvelope
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		resp, err := clientcontract.OK(req.RequestID, clientcontract.RuntimeCatalogResponse{
			SpaceID:       "docs",
			PluginCatalog: json.RawMessage(`{"version":1,"plugins":[]}`),
			GeneratedAt:   time.Now().UTC(),
		})
		if err != nil {
			t.Errorf("response: %v", err)
			return
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Errorf("marshal response: %v", err)
			return
		}
		_ = msg.Respond(data)
	}); err != nil {
		t.Fatalf("subscribe catalog: %v", err)
	}
	if err := conn.Flush(); err != nil {
		t.Fatalf("flush responder: %v", err)
	}

	got, err := FetchRuntimeCatalog(context.Background(), Config{
		URL:      srv.ClientURL(),
		Username: "runtime",
		Password: "secret",
		SpaceID:  "docs",
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("fetch catalog: %v", err)
	}
	if got.SpaceID != "docs" || string(got.PluginCatalog) != `{"version":1,"plugins":[]}` {
		t.Fatalf("catalog = %#v", got)
	}
}

func startCatalogTestServer(t *testing.T) *natsserver.Server {
	t.Helper()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host:     "127.0.0.1",
		Port:     -1,
		Username: "runtime",
		Password: "secret",
		NoLog:    true,
		NoSigs:   true,
	})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server did not become ready")
	}
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})
	return srv
}
