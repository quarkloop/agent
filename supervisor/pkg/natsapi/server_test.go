package natsapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	event "github.com/quarkloop/pkg/event"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/events"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/space/fsstore"
)

func TestSpaceAndSessionContracts(t *testing.T) {
	fixture := startFixture(t)

	createSpace := requestPayload[clientcontract.SpaceInfo](t, fixture.client, clientcontract.SubjectSpaceCreate, clientcontract.CreateSpaceRequest{
		Name:       "docs",
		Quarkfile:  spacemodel.DefaultQuarkfile("docs"),
		WorkingDir: filepath.Join(t.TempDir(), "workspace"),
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
		Name:      "docs",
		Quarkfile: spacemodel.DefaultQuarkfile("docs"),
	})
	assertCatalogEvent(t, catalogEvents, "docs", "quarkfile_updated")

	runtimeCredential := requestPayload[clientcontract.SpaceCredentialResponse](t, fixture.client, clientcontract.SubjectRuntimeCredential, clientcontract.SpaceCredentialRequest{SpaceID: "docs"})
	if runtimeCredential.Credential.Username == "" || runtimeCredential.Credential.Role != "runtime" {
		t.Fatalf("runtime credential = %#v", runtimeCredential.Credential)
	}

	listSpaces := requestPayload[clientcontract.ListSpacesResponse](t, fixture.client, clientcontract.SubjectSpaceList, struct{}{})
	if len(listSpaces.Spaces) != 1 || listSpaces.Spaces[0].Name != "docs" {
		t.Fatalf("spaces = %#v", listSpaces.Spaces)
	}

	_ = requestPayload[struct{}](t, fixture.client, clientcontract.SubjectKBSet, clientcontract.KBSetRequest{
		SpaceID:   "docs",
		Namespace: "config",
		Key:       "model",
		Value:     []byte("openrouter"),
	})
	kbValue := requestPayload[clientcontract.KBValueResponse](t, fixture.client, clientcontract.SubjectKBGet, clientcontract.KBRefRequest{
		SpaceID:   "docs",
		Namespace: "config",
		Key:       "model",
	})
	if string(kbValue.Value) != "openrouter" {
		t.Fatalf("kb value = %q", kbValue.Value)
	}
	kbKeys := requestPayload[clientcontract.KBListResponse](t, fixture.client, clientcontract.SubjectKBList, clientcontract.KBListRequest{
		SpaceID:   "docs",
		Namespace: "config",
	})
	if len(kbKeys.Keys) != 1 || kbKeys.Keys[0] != "model" {
		t.Fatalf("kb keys = %#v", kbKeys.Keys)
	}
	_ = requestPayload[struct{}](t, fixture.client, clientcontract.SubjectKBDelete, clientcontract.KBRefRequest{
		SpaceID:   "docs",
		Namespace: "config",
		Key:       "model",
	})

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

func TestCreateSpaceRunsRequiredPluginBootstrap(t *testing.T) {
	bootstrapper := &recordingSpaceBootstrapper{}
	fixture := startFixtureWithOptions(t, WithSpaceBootstrapper(bootstrapper))
	_ = requestPayload[clientcontract.SpaceInfo](t, fixture.client, clientcontract.SubjectSpaceCreate, clientcontract.CreateSpaceRequest{
		Name:       "docs",
		Quarkfile:  spacemodel.DefaultQuarkfile("docs"),
		WorkingDir: filepath.Join(t.TempDir(), "workspace"),
	})
	if len(bootstrapper.spaces) != 1 || bootstrapper.spaces[0] != "docs" {
		t.Fatalf("bootstrapped spaces = %#v", bootstrapper.spaces)
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

	store, err := fsstore.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	bus := events.NewBus()
	controlCredential, err := hub.ControlCredential()
	if err != nil {
		t.Fatalf("control credential: %v", err)
	}
	defaultOptions := []Option{WithServiceInspector(fakeServiceInspector{}), WithCatalogResolver(fakeCatalogResolver{})}
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

type recordingSpaceBootstrapper struct {
	spaces []string
}

func (b *recordingSpaceBootstrapper) BootstrapSpace(_ context.Context, spaceID string) error {
	b.spaces = append(b.spaces, spaceID)
	return nil
}

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

func requestPayload[T any](t *testing.T, client *nats.Conn, subject string, payload any) T {
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
	if resp.Status != "ok" {
		t.Fatalf("response error: %#v", resp.Error)
	}
	var out T
	if err := resp.DecodePayload(&out); err != nil {
		t.Fatalf("decode response payload: %v", err)
	}
	return out
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
