package natsapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	event "github.com/quarkloop/pkg/event"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/events"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/space"
)

func TestSpaceAndSessionContracts(t *testing.T) {
	fixture := startFixture(t)

	createSpace := requestPayload[clientcontract.SpaceInfo](t, fixture.client, clientcontract.SubjectSpaceCreate, clientcontract.CreateSpaceRequest{
		Config: spaceConfig(t, "docs", filepath.Join(t.TempDir(), "workspace")),
	})
	if createSpace.Name != "docs" {
		t.Fatalf("space name = %q", createSpace.Name)
	}

	accountName, err := natshub.SpaceAccountName("docs")
	if err != nil {
		t.Fatalf("space account name: %v", err)
	}
	if !hasAccount(fixture.hub.Config().Accounts, accountName) {
		t.Fatalf("space account %q was not provisioned", accountName)
	}

	spaceCredential := requestPayload[clientcontract.SpaceCredentialResponse](t, fixture.client, clientcontract.SubjectSpaceCredential, clientcontract.SpaceCredentialRequest{SpaceID: "docs"})
	if spaceCredential.Credential.Username == "" || spaceCredential.Credential.Password == "" {
		t.Fatalf("space credential = %#v", spaceCredential.Credential)
	}
	if spaceCredential.Credential.URL != "" {
		t.Fatalf("space credential leaks supervisor-local endpoint = %q", spaceCredential.Credential.URL)
	}
	spaceClient, err := nats.Connect(
		fixture.hub.Endpoints().ClientURL,
		nats.UserInfo(spaceCredential.Credential.Username, spaceCredential.Credential.Password),
		nats.Name("natsapi-space-client"),
		nats.Timeout(time.Second),
	)
	if err != nil {
		t.Fatalf("connect with space credential: %v", err)
	}
	t.Cleanup(spaceClient.Close)
	catalogEvents := make(chan clientcontract.RuntimeCatalogEvent, 1)
	if _, err := spaceClient.Subscribe(clientcontract.SubjectCatalogRuntimeEvents, func(msg *nats.Msg) {
		var event clientcontract.RuntimeCatalogEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			t.Errorf("decode catalog event: %v", err)
			return
		}
		catalogEvents <- event
	}); err != nil {
		t.Fatalf("subscribe catalog events: %v", err)
	}
	if err := spaceClient.Flush(); err != nil {
		t.Fatalf("flush catalog event subscription: %v", err)
	}
	_ = requestPayload[clientcontract.SpaceInfo](t, fixture.client, clientcontract.SubjectSpaceUpdate, clientcontract.UpdateSpaceRequest{
		Config: spaceConfig(t, "docs", filepath.Join(t.TempDir(), "workspace")),
	})
	assertCatalogEvent(t, catalogEvents, "docs", "space_config_updated")

	runtimeCredential := requestPayload[clientcontract.SpaceCredentialResponse](t, fixture.client, clientcontract.SubjectRuntimeCredential, clientcontract.SpaceCredentialRequest{SpaceID: "docs"})
	if runtimeCredential.Credential.Username == "" || runtimeCredential.Credential.Role != "runtime" {
		t.Fatalf("runtime credential = %#v", runtimeCredential.Credential)
	}
	if runtimeCredential.Credential.URL != "" {
		t.Fatalf("runtime credential leaks supervisor-local endpoint = %q", runtimeCredential.Credential.URL)
	}

	listSpaces := requestPayload[clientcontract.ListSpacesResponse](t, fixture.client, clientcontract.SubjectSpaceList, struct{}{})
	if len(listSpaces.Spaces) != 1 || listSpaces.Spaces[0].Name != "docs" {
		t.Fatalf("spaces = %#v", listSpaces.Spaces)
	}

	plugins := requestPayload[clientcontract.ListPluginsResponse](t, fixture.client, clientcontract.SubjectPluginList, clientcontract.ListPluginsRequest{
		SpaceID: "docs",
	})
	if len(plugins.Plugins) != 0 {
		t.Fatalf("plugins = %#v", plugins.Plugins)
	}

	services := requestPayload[clientcontract.ListServicesResponse](t, fixture.client, clientcontract.SubjectServiceList, clientcontract.ListServicesRequest{SpaceID: "docs"})
	if len(services.Services) != 1 || services.Services[0].Name != "indexer" {
		t.Fatalf("services = %#v", services.Services)
	}
	catalog := requestPayload[clientcontract.RuntimeCatalogResponse](t, fixture.client, clientcontract.SubjectCatalogRuntimeGet, clientcontract.RuntimeCatalogRequest{SpaceID: "docs"})
	if catalog.SpaceID != "docs" || string(catalog.PluginCatalog) != `{"version":1,"plugins":[]}` {
		t.Fatalf("runtime catalog = %#v", catalog)
	}
	service := requestPayload[clientcontract.ServiceInfo](t, fixture.client, clientcontract.SubjectServiceInspect, clientcontract.InspectServiceRequest{
		SpaceID: "docs",
		Service: "indexer",
	})
	if service.Status != clientcontract.ServiceStatusReady {
		t.Fatalf("service = %#v", service)
	}

	sessionEvents, unsubscribe := fixture.events.Subscribe("docs")
	defer unsubscribe()
	created := requestPayload[clientcontract.SessionInfo](t, fixture.client, clientcontract.SubjectSessionCreate, clientcontract.CreateSessionRequest{
		SpaceID: "docs",
		Type:    clientcontract.SessionTypeChat,
		Title:   "research",
	})
	if created.ID == "" {
		t.Fatal("session id is empty")
	}
	assertEvent(t, sessionEvents, events.SessionCreated)

	got := requestPayload[clientcontract.SessionInfo](t, fixture.client, clientcontract.SubjectSessionGet, clientcontract.SessionRefRequest{
		SpaceID:   "docs",
		SessionID: created.ID,
	})
	if got.ID != created.ID || got.Title != "research" {
		t.Fatalf("session = %#v", got)
	}

	credential := requestPayload[clientcontract.SessionCredentialResponse](t, fixture.client, clientcontract.SubjectSessionCredential, clientcontract.SessionCredentialRequest{
		SpaceID:   "docs",
		SessionID: created.ID,
	})
	if credential.Credential.Username == "" || credential.Credential.Password == "" {
		t.Fatalf("session credential = %#v", credential.Credential)
	}
	sessionClient, err := nats.Connect(
		fixture.hub.Endpoints().ClientURL,
		nats.UserInfo(credential.Credential.Username, credential.Credential.Password),
		nats.Name("natsapi-session-client"),
		nats.Timeout(time.Second),
	)
	if err != nil {
		t.Fatalf("connect with session credential: %v", err)
	}
	t.Cleanup(sessionClient.Close)

	listSessions := requestPayload[clientcontract.ListSessionsResponse](t, fixture.client, clientcontract.SubjectSessionList, clientcontract.ListSessionsRequest{SpaceID: "docs"})
	if len(listSessions.Sessions) != 1 || listSessions.Sessions[0].ID != created.ID {
		t.Fatalf("sessions = %#v", listSessions.Sessions)
	}

	_ = requestPayload[struct{}](t, fixture.client, clientcontract.SubjectSessionDelete, clientcontract.SessionRefRequest{
		SpaceID:   "docs",
		SessionID: created.ID,
	})
	assertEvent(t, sessionEvents, events.SessionDeleted)
}

func TestInvalidEnvelopeReturnsBoundaryError(t *testing.T) {
	fixture := startFixture(t)
	reply, err := fixture.client.Request(clientcontract.SubjectSpaceList, []byte(`{"version":"bad"}`), time.Second)
	if err != nil {
		t.Fatalf("request invalid envelope: %v", err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "error" || resp.Error == nil || resp.Error.Category != "invalid_argument" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestAuditContractsUseSupervisorOwnedStore(t *testing.T) {
	fixture := startFixture(t)
	js, err := fixture.client.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	event := natskit.ServiceCallEvent{
		Type: "service_call", ServiceCallID: "ref-1", ReferenceID: "ref-1", AuditRef: "ref-1",
		SpaceID: "docs", SessionID: "session-1", RunID: "run-1", Service: "indexer",
		Function: "query_context", Subject: "svc.indexer.v1.query_context", Status: "ok",
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal audit event: %v", err)
	}
	if _, err := js.Publish(natskit.ServiceCallRecordSubject("audit", "docs", "ref-1"), data); err != nil {
		t.Fatalf("publish audit record: %v", err)
	}

	record := requestPayload[clientcontract.AuditRecord](t, fixture.client, clientcontract.SubjectAuditGet, clientcontract.AuditGetRequest{SpaceID: "docs", ReferenceID: "ref-1"})
	if record.ReferenceID != "ref-1" || record.Service != "indexer" {
		t.Fatalf("record = %+v", record)
	}
	page := requestPayload[clientcontract.AuditListResponse](t, fixture.client, clientcontract.SubjectAuditList, clientcontract.AuditListRequest{SpaceID: "docs", RunID: "run-1", Limit: 10})
	if len(page.Records) != 1 || page.Records[0].Function != "query_context" {
		t.Fatalf("page = %+v", page)
	}
	retention := requestPayload[clientcontract.AuditRetentionResponse](t, fixture.client, clientcontract.SubjectAuditRetention, struct{}{})
	if retention.MaxAgeSeconds <= 0 || retention.MaxMessages <= 0 {
		t.Fatalf("retention = %+v", retention)
	}
	resp := requestEnvelope(t, fixture.client, clientcontract.SubjectAuditGet, clientcontract.AuditGetRequest{SpaceID: "docs", ReferenceID: "missing"})
	if resp.Error == nil || resp.Error.Category != "not_found" {
		t.Fatalf("missing response = %+v", resp)
	}
}

type fixture struct {
	hub    *natshub.Hub
	client *nats.Conn
	events *events.Bus
}

func startFixture(t *testing.T) fixture {
	return startFixtureWithOptions(t)
}

func startFixtureWithOptions(t *testing.T, opts ...Option) fixture {
	t.Helper()
	ctx := context.Background()
	cfg := natshub.DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = natsserver.RANDOM_PORT
	cfg.WebSocket.Enabled = false
	cfg.Monitoring.Enabled = false
	cfg.ReadyTimeout = 5 * time.Second
	cfg.NoLog = true

	hub, err := natshub.New(cfg)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if err := hub.Start(ctx); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hub.Stop(shutdownCtx)
	})

	store := newFixtureSpaceStore(t.TempDir())
	bus := events.NewBus()
	controlCredential, err := hub.ControlCredential()
	if err != nil {
		t.Fatalf("control credential: %v", err)
	}
	defaultOptions := []Option{
		WithServiceInspector(fakeServiceInspector{}),
		WithCatalogResolver(fakeCatalogResolver{}),
		WithPluginController(fakePluginController{}),
	}
	defaultOptions = append(defaultOptions, opts...)
	apiServer, err := Start(ctx, Config{
		URL:      hub.Endpoints().ClientURL,
		Username: controlCredential.Username,
		Password: controlCredential.Password,
	}, store, bus, hub, defaultOptions...)
	if err != nil {
		t.Fatalf("start nats api: %v", err)
	}
	t.Cleanup(apiServer.Close)

	client, err := nats.Connect(
		hub.Endpoints().ClientURL,
		nats.UserInfo(controlCredential.Username, controlCredential.Password),
		nats.Name("natsapi-test-client"),
		nats.Timeout(time.Second),
	)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(client.Close)

	return fixture{hub: hub, client: client, events: bus}
}

func spaceConfig(t *testing.T, name, workDir string) []byte {
	t.Helper()
	data, err := spacemodel.MarshalConfig(spacemodel.NewConfig(name, workDir))
	if err != nil {
		t.Fatalf("marshal space config: %v", err)
	}
	return data
}

// fixtureSpaceStore isolates NATS control-handler tests from Space service
// transport tests. remotestore tests cover the service-function delegation.
type fixtureSpaceStore struct {
	mu      sync.Mutex
	configs map[string][]byte
	records map[string][]byte
}

func newFixtureSpaceStore(_ string) *fixtureSpaceStore {
	return &fixtureSpaceStore{configs: make(map[string][]byte), records: make(map[string][]byte)}
}

func (s *fixtureSpaceStore) Create(data []byte) (*space.Space, error) {
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	if err := spacemodel.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.configs[cfg.Name]; exists {
		return nil, space.ErrAlreadyExists
	}
	s.configs[cfg.Name] = append([]byte(nil), data...)
	return space.FromConfig(*cfg), nil
}

func (s *fixtureSpaceStore) UpdateConfig(data []byte) (*space.Space, error) {
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	if err := spacemodel.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.configs[cfg.Name]; !exists {
		return nil, space.NewNotFoundError(cfg.Name)
	}
	s.configs[cfg.Name] = append([]byte(nil), data...)
	return space.FromConfig(*cfg), nil
}

func (s *fixtureSpaceStore) Get(name string) (*space.Space, error) {
	data, err := s.Config(name)
	if err != nil {
		return nil, err
	}
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	return space.FromConfig(*cfg), nil
}

func (s *fixtureSpaceStore) List() ([]*space.Space, error) {
	s.mu.Lock()
	names := make([]string, 0, len(s.configs))
	for name := range s.configs {
		names = append(names, name)
	}
	s.mu.Unlock()
	sort.Strings(names)
	out := make([]*space.Space, 0, len(names))
	for _, name := range names {
		item, err := s.Get(name)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *fixtureSpaceStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.configs[name]; !exists {
		return space.NewNotFoundError(name)
	}
	delete(s.configs, name)
	return nil
}

func (s *fixtureSpaceStore) Config(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, exists := s.configs[name]
	if !exists {
		return nil, space.NewNotFoundError(name)
	}
	return append([]byte(nil), data...), nil
}

func (s *fixtureSpaceStore) PutRecord(name, namespace, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[name+"/"+namespace+"/"+key] = append([]byte(nil), data...)
	return nil
}

func (s *fixtureSpaceStore) GetRecord(name, namespace, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, exists := s.records[name+"/"+namespace+"/"+key]
	if !exists {
		return nil, space.NewNotFoundError(key)
	}
	return append([]byte(nil), data...), nil
}

func (s *fixtureSpaceStore) ListRecords(name, namespace string) ([][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := name + "/" + namespace + "/"
	out := make([][]byte, 0)
	for key, data := range s.records {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			out = append(out, append([]byte(nil), data...))
		}
	}
	return out, nil
}

func (s *fixtureSpaceStore) DeleteRecord(name, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recordKey := name + "/" + namespace + "/" + key
	if _, exists := s.records[recordKey]; !exists {
		return space.NewNotFoundError(key)
	}
	delete(s.records, recordKey)
	return nil
}

func (s *fixtureSpaceStore) Doctor(name string) (space.DoctorResult, error) {
	if _, err := s.Config(name); err != nil {
		return space.DoctorResult{}, err
	}
	return space.DoctorResult{OK: true}, nil
}

var _ space.Store = (*fixtureSpaceStore)(nil)

type fakeServiceInspector struct{}

func (fakeServiceInspector) InspectServices(context.Context, string) ([]clientcontract.ServiceInfo, error) {
	return []clientcontract.ServiceInfo{{
		Name:    "indexer",
		Status:  clientcontract.ServiceStatusReady,
		Version: "1.0.0",
	}}, nil
}

type fakeCatalogResolver struct{}

func (fakeCatalogResolver) RuntimeCatalogSnapshot(context.Context, string) (clientcontract.RuntimeCatalogResponse, error) {
	return clientcontract.RuntimeCatalogResponse{
		SpaceID:       "docs",
		PluginCatalog: json.RawMessage(`{"version":1,"plugins":[]}`),
		GeneratedAt:   time.Now().UTC(),
	}, nil
}

type fakePluginController struct{}

func (fakePluginController) ListSpacePlugins(context.Context, string, string) ([]pluginmanager.InstalledPlugin, error) {
	return nil, nil
}

func (fakePluginController) GetSpacePlugin(context.Context, string, string) (pluginmanager.InstalledPlugin, error) {
	return pluginmanager.InstalledPlugin{}, space.NewNotFoundError("plugin")
}

func (fakePluginController) InstallSpacePlugin(context.Context, string, string) (pluginmanager.InstalledPlugin, error) {
	return pluginmanager.InstalledPlugin{}, nil
}

func (fakePluginController) UninstallSpacePlugin(context.Context, string, string) error {
	return nil
}

func (fakePluginController) SearchPlugins(context.Context, string) ([]pluginmanager.PluginSearchItem, error) {
	return nil, nil
}

func (fakePluginController) HubPluginInfo(context.Context, string) (*pluginmanager.HubPlugin, error) {
	return nil, space.NewNotFoundError("plugin")
}

func requestPayload[T any](t *testing.T, client *nats.Conn, subject string, payload any) T {
	t.Helper()
	resp := requestEnvelope(t, client, subject, payload)
	if resp.Status != "ok" {
		t.Fatalf("response error: %#v", resp.Error)
	}
	var out T
	if err := resp.DecodePayload(&out); err != nil {
		t.Fatalf("decode response payload: %v", err)
	}
	return out
}

func requestEnvelope(t *testing.T, client *nats.Conn, subject string, payload any) clientcontract.ResponseEnvelope {
	t.Helper()
	req, err := clientcontract.NewRequest("req-"+subject, "", payload)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	reply, err := client.Request(subject, data, time.Second)
	if err != nil {
		t.Fatalf("request %s: %v", subject, err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &resp); err != nil {
		t.Fatalf("decode response envelope: %v", err)
	}
	if err := resp.Validate(); err != nil {
		t.Fatalf("validate response: %v", err)
	}
	return resp
}

func assertEvent(t *testing.T, events <-chan event.Event, want event.Kind) {
	t.Helper()
	select {
	case got := <-events:
		if got.Kind != want {
			t.Fatalf("event kind = %q, want %q", got.Kind, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", want)
	}
}

func assertCatalogEvent(t *testing.T, events <-chan clientcontract.RuntimeCatalogEvent, spaceID, reason string) {
	t.Helper()
	select {
	case got := <-events:
		if got.SpaceID != spaceID || got.Reason != reason || got.GeneratedAt.IsZero() {
			t.Fatalf("catalog event = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for catalog event %s", reason)
	}
}

func hasAccount(accounts []natshub.AccountConfig, name string) bool {
	for _, account := range accounts {
		if account.Name == name {
			return true
		}
	}
	return false
}
