package natsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/events"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/sessions"
	"github.com/quarkloop/supervisor/pkg/space"
	spacestore "github.com/quarkloop/supervisor/pkg/space/store"
)

type SpaceProvisioner interface {
	ProvisionSpace(spaceID string) (natshub.SpaceCredentials, error)
}

type CredentialIssuer interface {
	IssueUserCredential(spaceID string) (natshub.Credential, error)
	IssueRuntimeCredential(spaceID string) (natshub.Credential, error)
	IssueSessionCredential(spaceID, sessionID string) (natshub.Credential, error)
}

type ServiceInspector interface {
	InspectServices(ctx context.Context, spaceID string) ([]clientcontract.ServiceInfo, error)
}

type CatalogResolver interface {
	RuntimeCatalogSnapshot(ctx context.Context, spaceID string) (clientcontract.RuntimeCatalogResponse, error)
}

type SpaceBootstrapper interface {
	BootstrapSpace(ctx context.Context, spaceID string) error
}

type AuditReader interface {
	GetAuditRecord(ctx context.Context, spaceID, referenceID string) (natshub.StoredAuditRecord, error)
	ListAuditRecords(ctx context.Context, filter natshub.AuditFilter) (natshub.AuditPage, error)
	AuditRetention() natshub.AuditRetention
}

type Option func(*Server)

func WithServiceInspector(inspector ServiceInspector) Option {
	return func(s *Server) {
		s.serviceInspector = inspector
	}
}

func WithCatalogResolver(resolver CatalogResolver) Option {
	return func(s *Server) {
		s.catalogResolver = resolver
	}
}

func WithCredentialIssuer(issuer CredentialIssuer) Option {
	return func(s *Server) {
		s.credentialIssuer = issuer
	}
}

func WithSpaceBootstrapper(bootstrapper SpaceBootstrapper) Option {
	return func(s *Server) {
		s.spaceBootstrapper = bootstrapper
	}
}

func WithAuditReader(reader AuditReader) Option {
	return func(s *Server) {
		s.auditReader = reader
	}
}

type Server struct {
	client            *natskit.Client
	url               string
	store             space.Store
	events            *events.Bus
	provisioner       SpaceProvisioner
	credentialIssuer  CredentialIssuer
	serviceInspector  ServiceInspector
	catalogResolver   CatalogResolver
	spaceBootstrapper SpaceBootstrapper
	auditReader       AuditReader
	subs              []*natskit.Subscription
}

type Config struct {
	URL      string
	Username string
	Password string
}

func Start(ctx context.Context, cfg Config, store space.Store, bus *events.Bus, provisioner SpaceProvisioner, opts ...Option) (*Server, error) {
	if store == nil {
		return nil, fmt.Errorf("space store is required")
	}
	if bus == nil {
		bus = events.NewBus()
	}
	client, err := natskit.Connect(ctx, natskit.Config{
		URL: cfg.URL, Username: cfg.Username, Password: cfg.Password,
		Name: "quark-supervisor-control-api", Timeout: 5 * time.Second,
		ReconnectWait: 250 * time.Millisecond, MaxReconnects: 10,
	})
	if err != nil {
		return nil, fmt.Errorf("connect nats api: %w", err)
	}
	server := &Server{
		client:           client,
		url:              cfg.URL,
		store:            store,
		events:           bus,
		provisioner:      provisioner,
		credentialIssuer: credentialIssuerFromProvisioner(provisioner),
		auditReader:      auditReaderFromProvisioner(provisioner),
	}
	for _, opt := range opts {
		opt(server)
	}
	if err := server.subscribe(); err != nil {
		server.Close()
		return nil, err
	}
	return server, nil
}

func credentialIssuerFromProvisioner(provisioner SpaceProvisioner) CredentialIssuer {
	issuer, _ := provisioner.(CredentialIssuer)
	return issuer
}

func auditReaderFromProvisioner(provisioner SpaceProvisioner) AuditReader {
	reader, _ := provisioner.(AuditReader)
	return reader
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	for _, sub := range s.subs {
		_ = sub.Unsubscribe()
	}
	if s.client != nil {
		s.client.Close()
	}
}

func (s *Server) subscribe() error {
	handlers := map[string]func(clientcontract.RequestEnvelope) (any, error){
		clientcontract.SubjectSpaceCreate:       s.createSpace,
		clientcontract.SubjectSpaceList:         s.listSpaces,
		clientcontract.SubjectSpaceGet:          s.getSpace,
		clientcontract.SubjectSpaceUpdate:       s.updateSpace,
		clientcontract.SubjectSpaceDelete:       s.deleteSpace,
		clientcontract.SubjectSpaceQuarkfile:    s.getQuarkfile,
		clientcontract.SubjectSpaceDoctor:       s.doctor,
		clientcontract.SubjectSpaceCredential:   s.spaceCredential,
		clientcontract.SubjectRuntimeCredential: s.runtimeCredential,
		clientcontract.SubjectSessionCreate:     s.createSession,
		clientcontract.SubjectSessionList:       s.listSessions,
		clientcontract.SubjectSessionGet:        s.getSession,
		clientcontract.SubjectSessionDelete:     s.deleteSession,
		clientcontract.SubjectSessionCredential: s.sessionCredential,
		clientcontract.SubjectKBGet:             s.getKB,
		clientcontract.SubjectKBSet:             s.setKB,
		clientcontract.SubjectKBDelete:          s.deleteKB,
		clientcontract.SubjectKBList:            s.listKB,
		clientcontract.SubjectPluginList:        s.listPlugins,
		clientcontract.SubjectPluginGet:         s.getPlugin,
		clientcontract.SubjectPluginInstall:     s.installPlugin,
		clientcontract.SubjectPluginUninstall:   s.uninstallPlugin,
		clientcontract.SubjectPluginSearch:      s.searchPlugins,
		clientcontract.SubjectPluginHubInfo:     s.hubPluginInfo,
		clientcontract.SubjectServiceList:       s.listServices,
		clientcontract.SubjectServiceInspect:    s.inspectService,
		clientcontract.SubjectServiceDoctor:     s.serviceDoctor,
		clientcontract.SubjectCatalogRuntimeGet: s.runtimeCatalog,
		clientcontract.SubjectAuditGet:          s.getAuditRecord,
		clientcontract.SubjectAuditList:         s.listAuditRecords,
		clientcontract.SubjectAuditRetention:    s.auditRetention,
	}
	for subject, handler := range handlers {
		subject := subject
		handler := handler
		sub, err := s.client.Respond(subject, "q.supervisor.control", 5*time.Second, func(_ context.Context, msg natskit.Message) ([]byte, error) {
			return s.handle(msg, handler), nil
		})
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
		s.subs = append(s.subs, sub)
	}
	return s.client.Flush(context.Background())
}

func (s *Server) handle(msg natskit.Message, handler func(clientcontract.RequestEnvelope) (any, error)) []byte {
	var req clientcontract.RequestEnvelope
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return encodeResponse(clientcontract.Error("", string(boundary.InvalidArgument), "invalid request envelope: "+err.Error()))
	}
	if err := req.Validate(); err != nil {
		return encodeResponse(clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
	}
	payload, err := handler(req)
	if err != nil {
		return encodeResponse(clientcontract.Error(req.RequestID, string(errorCategory(msg.Subject, err)), err.Error()))
	}
	resp, err := clientcontract.OK(req.RequestID, payload)
	if err != nil {
		return encodeResponse(clientcontract.Error(req.RequestID, string(boundary.Internal), err.Error()))
	}
	return encodeResponse(resp)
}

func errorCategory(operation string, err error) boundary.Category {
	switch {
	case spacestore.IsNotFound(err), errors.Is(err, sessions.ErrNotFound), errors.Is(err, natshub.ErrAuditRecordNotFound):
		return boundary.NotFound
	case errors.Is(err, spacestore.ErrAlreadyExists):
		return boundary.Conflict
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
		return boundary.NotFound
	default:
		return boundary.FromError(boundary.Supervisor, operation, err).Category
	}
}

func encodeResponse(resp clientcontract.ResponseEnvelope) []byte {
	data, err := json.Marshal(resp)
	if err != nil {
		data = []byte(`{"version":"v1","request_id":"","status":"error","error":{"category":"internal","message":"marshal response"}}`)
	}
	return data
}
