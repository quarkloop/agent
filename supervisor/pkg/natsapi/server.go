package natsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
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

type Server struct {
	conn             *nats.Conn
	url              string
	store            space.Store
	events           *events.Bus
	provisioner      SpaceProvisioner
	credentialIssuer CredentialIssuer
	serviceInspector ServiceInspector
	catalogResolver  CatalogResolver
	subs             []*nats.Subscription
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
	conn, err := nats.Connect(
		cfg.URL,
		nats.UserInfo(cfg.Username, cfg.Password),
		nats.Name("quark-supervisor-control-api"),
		nats.Timeout(5*time.Second),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(250*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats api: %w", err)
	}
	if err := verifyConnection(ctx, conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("verify nats api connection: %w", err)
	}
	server := &Server{
		conn:             conn,
		url:              cfg.URL,
		store:            store,
		events:           bus,
		provisioner:      provisioner,
		credentialIssuer: credentialIssuerFromProvisioner(provisioner),
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

func verifyConnection(ctx context.Context, conn *nats.Conn) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); ok {
		return conn.FlushWithContext(ctx)
	}
	return conn.FlushTimeout(5 * time.Second)
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	for _, sub := range s.subs {
		_ = sub.Unsubscribe()
	}
	if s.conn != nil {
		s.conn.Close()
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
	}
	for subject, handler := range handlers {
		subject := subject
		handler := handler
		sub, err := s.conn.QueueSubscribe(subject, "q.supervisor.control", func(msg *nats.Msg) {
			s.handle(msg, handler)
		})
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
		s.subs = append(s.subs, sub)
	}
	return s.conn.Flush()
}

func (s *Server) handle(msg *nats.Msg, handler func(clientcontract.RequestEnvelope) (any, error)) {
	var req clientcontract.RequestEnvelope
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		respond(msg, clientcontract.Error("", string(boundary.InvalidArgument), "invalid request envelope: "+err.Error()))
		return
	}
	if err := req.Validate(); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	payload, err := handler(req)
	if err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(errorCategory(msg.Subject, err)), err.Error()))
		return
	}
	resp, err := clientcontract.OK(req.RequestID, payload)
	if err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.Internal), err.Error()))
		return
	}
	respond(msg, resp)
}

func errorCategory(operation string, err error) boundary.Category {
	switch {
	case spacestore.IsNotFound(err), errors.Is(err, sessions.ErrNotFound):
		return boundary.NotFound
	case errors.Is(err, spacestore.ErrAlreadyExists):
		return boundary.Conflict
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
		return boundary.NotFound
	default:
		return boundary.FromError(boundary.Supervisor, operation, err).Category
	}
}

func respond(msg *nats.Msg, resp clientcontract.ResponseEnvelope) {
	data, err := json.Marshal(resp)
	if err != nil {
		data = []byte(`{"version":"v1","request_id":"","status":"error","error":{"category":"internal","message":"marshal response"}}`)
	}
	_ = msg.Respond(data)
}
