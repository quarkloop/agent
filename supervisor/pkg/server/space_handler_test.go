package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func TestCreateSpaceProvisionsNATSAccount(t *testing.T) {
	srv := spaceRouteServer(t)
	body, err := json.Marshal(api.CreateSpaceRequest{
		Name:       "docs",
		Quarkfile:  spacemodel.DefaultQuarkfile("docs"),
		WorkingDir: filepath.Join(t.TempDir(), "workspace"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := srv.app.Test(newRequest(t, http.MethodPost, "/v1/spaces", string(body)))
	if err != nil {
		t.Fatalf("create space: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	accountName, err := natshub.SpaceAccountName("docs")
	if err != nil {
		t.Fatalf("space account name: %v", err)
	}
	cfg := srv.natsHub.Config()
	if !hasNATSAccount(cfg.Accounts, accountName) {
		t.Fatalf("account %q was not provisioned in %#v", accountName, cfg.Accounts)
	}
	plugins, err := srv.store.Plugins("docs")
	if err != nil {
		t.Fatalf("open plugin store: %v", err)
	}
	mainAgent, err := plugins.Get("quark-main")
	if err != nil {
		t.Fatalf("required main agent plugin was not installed: %v", err)
	}
	if mainAgent.Manifest.Type != plugin.TypeAgent {
		t.Fatalf("required plugin type = %q", mainAgent.Manifest.Type)
	}
}

func spaceRouteServer(t *testing.T) *Server {
	t.Helper()
	cfg := natshub.DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = 0
	cfg.WebSocket.Enabled = false
	cfg.Monitoring.Enabled = false
	cfg.NoLog = true
	srv, err := New(Config{
		SpacesDir:         t.TempDir(),
		NATS:              cfg,
		BundledPluginsDir: filepath.Join("..", "..", "..", "plugins"),
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return srv
}

func hasNATSAccount(accounts []natshub.AccountConfig, name string) bool {
	for _, account := range accounts {
		if account.Name == name {
			return true
		}
	}
	return false
}
