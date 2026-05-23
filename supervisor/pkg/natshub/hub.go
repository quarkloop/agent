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

	mu          sync.Mutex
	server      *natsserver.Server
	started     bool
	accounts    map[string]*natsserver.Account
	credentials *credentialRegistry
	spaces      map[string]SpaceCredentials
	issued      map[string]Credential
	imports     map[string][]ServiceFunctionRoute
	applied     map[string]map[string]struct{}
}

func New(cfg Config) (*Hub, error) {
	normalized, err := Normalize(cfg)
	if err != nil {
		return nil, err
	}
	return &Hub{
		cfg:     normalized,
		spaces:  make(map[string]SpaceCredentials),
		issued:  make(map[string]Credential),
		imports: make(map[string][]ServiceFunctionRoute),
		applied: make(map[string]map[string]struct{}),
	}, nil
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
	opts, configuredAccounts, credentials, err := buildOptionsAndRegistry(h.cfg)
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
	accounts, err := registeredAccounts(srv, configuredAccounts)
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		return err
	}
	if err := credentials.rebindAccounts(accounts); err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		return err
	}
	h.server = srv
	h.accounts = accounts
	h.credentials = credentials
	h.started = true
	h.applied = make(map[string]map[string]struct{})
	for spaceID, routes := range h.imports {
		space, ok := h.spaces[spaceID]
		if !ok {
			h.shutdownStartedServerLocked()
			return fmt.Errorf("space %q has service imports but no provisioned account", spaceID)
		}
		if err := h.applyServiceFunctionImportsLocked(space.Account, routes); err != nil {
			h.shutdownStartedServerLocked()
			return err
		}
	}
	return nil
}

func registeredAccounts(srv *natsserver.Server, configured map[string]*natsserver.Account) (map[string]*natsserver.Account, error) {
	out := make(map[string]*natsserver.Account, len(configured))
	for name := range configured {
		account, err := srv.LookupAccount(name)
		if err != nil {
			return nil, fmt.Errorf("lookup registered nats account %q: %w", name, err)
		}
		out[name] = account
	}
	return out, nil
}

func (h *Hub) shutdownStartedServerLocked() {
	srv := h.server
	h.server = nil
	h.started = false
	h.accounts = nil
	h.credentials = nil
	h.applied = make(map[string]map[string]struct{})
	if srv != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
	}
}

func (h *Hub) Stop(ctx context.Context) error {
	h.mu.Lock()
	srv := h.server
	h.server = nil
	h.started = false
	h.accounts = nil
	h.credentials = nil
	h.applied = make(map[string]map[string]struct{})
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

func (h *Hub) ControlCredential() (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, account := range h.cfg.Accounts {
		if account.Name != ControlAccountName {
			continue
		}
		if len(account.Users) == 0 {
			return Credential{}, errors.New("nats control account has no users")
		}
		user := cloneUserConfig(account.Users[0])
		return Credential{
			Username:    user.Name,
			Password:    user.Password,
			Account:     ControlAccountName,
			Role:        RoleSupervisor,
			Permissions: clonePermissions(user.Permissions),
		}, nil
	}
	return Credential{}, errors.New("nats control account is not configured")
}

func (h *Hub) ServerForTest() *natsserver.Server {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.server
}

func (h *Hub) ProvisionSpace(spaceID string) (SpaceCredentials, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.provisionSpaceLocked(spaceID)
}

func (h *Hub) IssueSessionCredential(spaceID, sessionID string) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return Credential{}, err
	}
	key := issuedCredentialKey(RoleSession, spaceID, sessionID)
	if existing, ok := h.issued[key]; ok {
		return cloneCredential(existing), nil
	}
	credential, err := sessionCredential(spaceID, space.Account, sessionID)
	if err != nil {
		return Credential{}, err
	}
	if err := h.registerCredentialLocked(credential); err != nil {
		return Credential{}, err
	}
	h.issued[key] = cloneCredential(credential)
	return cloneCredential(credential), nil
}

func (h *Hub) IssueUserCredential(spaceID string) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return Credential{}, err
	}
	key := issuedCredentialKey(RoleUser, spaceID, "user")
	if existing, ok := h.issued[key]; ok {
		return cloneCredential(existing), nil
	}
	credential, err := userCredential(spaceID, space.Account)
	if err != nil {
		return Credential{}, err
	}
	if err := h.registerCredentialLocked(credential); err != nil {
		return Credential{}, err
	}
	h.issued[key] = cloneCredential(credential)
	return cloneCredential(credential), nil
}

func (h *Hub) IssueRuntimeCredential(spaceID string) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return Credential{}, err
	}
	return cloneCredential(space.Runtime), nil
}

func (h *Hub) IssueAgentCredential(spaceID, agentID string) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return Credential{}, err
	}
	key := issuedCredentialKey(RoleAgent, spaceID, agentID)
	if existing, ok := h.issued[key]; ok {
		return cloneCredential(existing), nil
	}
	credential, err := agentCredential(spaceID, space.Account, agentID)
	if err != nil {
		return Credential{}, err
	}
	if err := h.registerCredentialLocked(credential); err != nil {
		return Credential{}, err
	}
	h.issued[key] = cloneCredential(credential)
	return cloneCredential(credential), nil
}

func (h *Hub) IssueServiceCredential(service string, routes []ServiceFunctionRoute) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	normalized, err := NormalizeServiceFunctionRoutes(routes)
	if err != nil {
		return Credential{}, err
	}
	key := issuedCredentialKey(RoleService, ControlAccountName, service)
	if existing, ok := h.issued[key]; ok {
		return cloneCredential(existing), nil
	}
	credential, err := serviceCredential(service, ControlAccountName, normalized)
	if err != nil {
		return Credential{}, err
	}
	if err := h.registerCredentialLocked(credential); err != nil {
		return Credential{}, err
	}
	h.issued[key] = cloneCredential(credential)
	return cloneCredential(credential), nil
}

func (h *Hub) ImportServiceFunctions(spaceID string, routes []ServiceFunctionRoute) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	normalized, err := NormalizeServiceFunctionRoutes(routes)
	if err != nil {
		return err
	}
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return err
	}
	h.imports[spaceID] = cloneServiceFunctionRoutes(normalized)
	if !h.started {
		return nil
	}
	return h.applyServiceFunctionImportsLocked(space.Account, normalized)
}

func (h *Hub) provisionSpaceLocked(spaceID string) (SpaceCredentials, error) {
	if h.cfg.Mode == ModeExternal {
		return SpaceCredentials{}, ErrProvisioningUnavailable
	}
	if existing, ok := h.spaces[spaceID]; ok {
		return cloneSpaceCredentials(existing), nil
	}
	accountName, err := SpaceAccountName(spaceID)
	if err != nil {
		return SpaceCredentials{}, err
	}
	runtimeCred, err := spaceCredential(spaceID, accountName, RoleRuntime, RuntimePermissions())
	if err != nil {
		return SpaceCredentials{}, err
	}
	serviceCred, err := spaceCredential(spaceID, accountName, RoleService, ServicePermissions())
	if err != nil {
		return SpaceCredentials{}, err
	}
	observabilityCred, err := spaceCredential(spaceID, ObservabilityAccountName, RoleObservability, ObservabilityPermissions(spaceID))
	if err != nil {
		return SpaceCredentials{}, err
	}
	space := SpaceCredentials{
		SpaceID:       spaceID,
		Account:       accountName,
		Runtime:       runtimeCred,
		Service:       serviceCred,
		Observability: observabilityCred,
	}
	if h.started {
		if _, err := h.accountLocked(accountName); err != nil {
			return SpaceCredentials{}, err
		}
		if err := h.registerCredentialLocked(runtimeCred); err != nil {
			return SpaceCredentials{}, err
		}
		if err := h.registerCredentialLocked(serviceCred); err != nil {
			return SpaceCredentials{}, err
		}
		if err := h.registerCredentialLocked(observabilityCred); err != nil {
			return SpaceCredentials{}, err
		}
	} else {
		h.cfg.Accounts = upsertAccountUsers(h.cfg.Accounts, AccountConfig{
			Name: accountName,
			Users: []UserConfig{
				userConfigFromCredential(runtimeCred),
				userConfigFromCredential(serviceCred),
			},
		})
		h.cfg.Accounts = appendUserToAccount(h.cfg.Accounts, ObservabilityAccountName, userConfigFromCredential(observabilityCred))
	}
	h.spaces[spaceID] = cloneSpaceCredentials(space)
	return cloneSpaceCredentials(space), nil
}

func (h *Hub) accountLocked(accountName string) (*natsserver.Account, error) {
	if account, ok := h.accounts[accountName]; ok {
		return account, nil
	}
	if h.server == nil {
		return nil, errors.New("embedded nats server is not started")
	}
	account, err := h.server.RegisterAccount(accountName)
	if err != nil {
		existing, lookupErr := h.server.LookupAccount(accountName)
		if lookupErr != nil {
			return nil, err
		}
		account = existing
	}
	if h.accounts == nil {
		h.accounts = make(map[string]*natsserver.Account)
	}
	h.accounts[accountName] = account
	return account, nil
}

func (h *Hub) registerCredentialLocked(credential Credential) error {
	if !h.started {
		h.cfg.Accounts = appendUserToAccount(h.cfg.Accounts, credential.Account, userConfigFromCredential(credential))
		return nil
	}
	account, err := h.accountLocked(credential.Account)
	if err != nil {
		return err
	}
	if h.credentials == nil {
		h.credentials = newCredentialRegistry()
	}
	if err := h.credentials.add(account, credential); err != nil {
		return err
	}
	h.cfg.Accounts = appendUserToAccount(h.cfg.Accounts, credential.Account, userConfigFromCredential(credential))
	return nil
}

func (h *Hub) applyServiceFunctionImportsLocked(spaceAccountName string, routes []ServiceFunctionRoute) error {
	controlAccount, err := h.accountLocked(ControlAccountName)
	if err != nil {
		return err
	}
	spaceAccount, err := h.accountLocked(spaceAccountName)
	if err != nil {
		return err
	}
	for _, route := range routes {
		key := route.ImportSubject + "\x00" + route.ExportSubject
		if _, ok := h.applied[spaceAccountName]; !ok {
			h.applied[spaceAccountName] = make(map[string]struct{})
		}
		if _, ok := h.applied[spaceAccountName][key]; ok {
			continue
		}
		if err := controlAccount.AddServiceExport(route.ExportSubject, []*natsserver.Account{spaceAccount}); err != nil {
			return fmt.Errorf("export service function %q: %w", route.ExportSubject, err)
		}
		if err := spaceAccount.AddServiceImport(controlAccount, route.ImportSubject, route.ExportSubject); err != nil {
			return fmt.Errorf("import service function %q into %q: %w", route.ImportSubject, spaceAccountName, err)
		}
		h.applied[spaceAccountName][key] = struct{}{}
	}
	return nil
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
