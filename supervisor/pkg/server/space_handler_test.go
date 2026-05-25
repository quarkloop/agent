package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/plugin"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/sessions"
	supervisorspace "github.com/quarkloop/supervisor/pkg/space"
	spacestore "github.com/quarkloop/supervisor/pkg/space/store"
)

func TestCreateSpaceProvisionsNATSAccount(t *testing.T) {
	srv := spaceRouteServer(t)
	body, err := json.Marshal(api.CreateSpaceRequest{
		Config: testSpaceConfig(t, "docs", filepath.Join(t.TempDir(), "workspace")),
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
	cfg.Client.Port = natsserver.RANDOM_PORT
	cfg.WebSocket.Enabled = false
	cfg.Monitoring.Enabled = false
	cfg.NoLog = true
	store := &routeSpaceStore{root: t.TempDir(), configs: make(map[string][]byte)}
	srv, err := New(Config{
		SpacesDir:         t.TempDir(),
		NATS:              cfg,
		BundledPluginsDir: filepath.Join("..", "..", "..", "plugins"),
		Store:             store,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return srv
}

func testSpaceConfig(t *testing.T, name, workingDir string) []byte {
	t.Helper()
	data, err := spacemodel.MarshalConfig(spacemodel.NewConfig(name, workingDir))
	if err != nil {
		t.Fatalf("marshal space config: %v", err)
	}
	return data
}

func hasNATSAccount(accounts []natshub.AccountConfig, name string) bool {
	for _, account := range accounts {
		if account.Name == name {
			return true
		}
	}
	return false
}

// routeSpaceStore keeps HTTP handler tests at the supervisor semantic
// boundary; NATS delegation is covered by remotestore tests.
type routeSpaceStore struct {
	root    string
	configs map[string][]byte
}

func (s *routeSpaceStore) Create(data []byte) (*supervisorspace.Space, error) {
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	if err := spacemodel.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	if _, exists := s.configs[cfg.Name]; exists {
		return nil, spacestore.ErrAlreadyExists
	}
	s.configs[cfg.Name] = append([]byte(nil), data...)
	return supervisorspace.FromConfig(*cfg), nil
}

func (s *routeSpaceStore) UpdateConfig(data []byte) (*supervisorspace.Space, error) {
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	if _, exists := s.configs[cfg.Name]; !exists {
		return nil, spacestore.NewNotFoundError(cfg.Name)
	}
	s.configs[cfg.Name] = append([]byte(nil), data...)
	return supervisorspace.FromConfig(*cfg), nil
}

func (s *routeSpaceStore) Get(name string) (*supervisorspace.Space, error) {
	data, err := s.Config(name)
	if err != nil {
		return nil, err
	}
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	return supervisorspace.FromConfig(*cfg), nil
}

func (s *routeSpaceStore) List() ([]*supervisorspace.Space, error) {
	out := make([]*supervisorspace.Space, 0, len(s.configs))
	for name := range s.configs {
		item, err := s.Get(name)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *routeSpaceStore) Delete(name string) error {
	if _, exists := s.configs[name]; !exists {
		return spacestore.NewNotFoundError(name)
	}
	delete(s.configs, name)
	return nil
}

func (s *routeSpaceStore) Config(name string) ([]byte, error) {
	data, exists := s.configs[name]
	if !exists {
		return nil, spacestore.NewNotFoundError(name)
	}
	return append([]byte(nil), data...), nil
}

func (s *routeSpaceStore) AgentEnvironment(string) ([]string, error) { return nil, nil }

func (s *routeSpaceStore) Plugins(name string) (*pluginmanager.Installer, error) {
	return pluginmanager.NewInstaller(filepath.Join(s.root, name, "plugins")), nil
}

func (s *routeSpaceStore) Sessions(name string) (*sessions.Store, error) {
	return sessions.Open(filepath.Join(s.root, name, "sessions"), name)
}

func (s *routeSpaceStore) ServiceStateDir(name, serviceName string) (string, error) {
	return filepath.Join(s.root, name, "services", serviceName), nil
}

func (s *routeSpaceStore) Doctor(name string) (api.DoctorResponse, error) {
	if _, err := s.Config(name); err != nil {
		return api.DoctorResponse{}, err
	}
	return api.DoctorResponse{OK: true}, nil
}

var _ supervisorspace.Store = (*routeSpaceStore)(nil)
