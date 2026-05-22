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
	event "github.com/quarkloop/pkg/event"
	plugin "github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/events"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/sessions"
	"github.com/quarkloop/supervisor/pkg/space"
	spacestore "github.com/quarkloop/supervisor/pkg/space/store"
)

type SpaceProvisioner interface {
	ProvisionSpace(spaceID string) (natshub.SpaceCredentials, error)
}

type Server struct {
	conn        *nats.Conn
	store       space.Store
	events      *events.Bus
	provisioner SpaceProvisioner
	subs        []*nats.Subscription
}

type Config struct {
	URL      string
	Username string
	Password string
}

func Start(ctx context.Context, cfg Config, store space.Store, bus *events.Bus, provisioner SpaceProvisioner) (*Server, error) {
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
	server := &Server{conn: conn, store: store, events: bus, provisioner: provisioner}
	if err := server.subscribe(); err != nil {
		server.Close()
		return nil, err
	}
	return server, nil
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
		clientcontract.SubjectSpaceCreate:     s.createSpace,
		clientcontract.SubjectSpaceList:       s.listSpaces,
		clientcontract.SubjectSpaceGet:        s.getSpace,
		clientcontract.SubjectSpaceUpdate:     s.updateSpace,
		clientcontract.SubjectSpaceDelete:     s.deleteSpace,
		clientcontract.SubjectSpaceQuarkfile:  s.getQuarkfile,
		clientcontract.SubjectSpaceDoctor:     s.doctor,
		clientcontract.SubjectSessionCreate:   s.createSession,
		clientcontract.SubjectSessionList:     s.listSessions,
		clientcontract.SubjectSessionGet:      s.getSession,
		clientcontract.SubjectSessionDelete:   s.deleteSession,
		clientcontract.SubjectKBGet:           s.getKB,
		clientcontract.SubjectKBSet:           s.setKB,
		clientcontract.SubjectKBDelete:        s.deleteKB,
		clientcontract.SubjectKBList:          s.listKB,
		clientcontract.SubjectPluginList:      s.listPlugins,
		clientcontract.SubjectPluginGet:       s.getPlugin,
		clientcontract.SubjectPluginInstall:   s.installPlugin,
		clientcontract.SubjectPluginUninstall: s.uninstallPlugin,
		clientcontract.SubjectPluginSearch:    s.searchPlugins,
		clientcontract.SubjectPluginHubInfo:   s.hubPluginInfo,
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

func (s *Server) createSpace(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.CreateSpaceRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	sp, err := s.store.Create(payload.Name, append([]byte(nil), payload.Quarkfile...), payload.WorkingDir)
	if err != nil {
		return nil, err
	}
	if s.provisioner != nil {
		if _, err := s.provisioner.ProvisionSpace(payload.Name); err != nil {
			if rollbackErr := s.store.Delete(payload.Name); rollbackErr != nil {
				return nil, fmt.Errorf("provision nats space account: %v; rollback space: %v", err, rollbackErr)
			}
			return nil, err
		}
	}
	return toContractSpace(sp), nil
}

func (s *Server) listSpaces(req clientcontract.RequestEnvelope) (any, error) {
	spaces, err := s.store.List()
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.SpaceInfo, 0, len(spaces))
	for _, sp := range spaces {
		out = append(out, toContractSpace(sp))
	}
	return clientcontract.ListSpacesResponse{Spaces: out}, nil
}

func (s *Server) getSpace(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.GetSpaceRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	sp, err := s.store.Get(payload.Name)
	if err != nil {
		return nil, err
	}
	return toContractSpace(sp), nil
}

func (s *Server) updateSpace(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.UpdateSpaceRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	sp, err := s.store.UpdateQuarkfile(payload.Name, append([]byte(nil), payload.Quarkfile...))
	if err != nil {
		return nil, err
	}
	s.events.Publish(event.Event{
		Space:   payload.Name,
		Kind:    events.QuarkfileUpdated,
		Payload: nil,
	})
	return toContractSpace(sp), nil
}

func (s *Server) deleteSpace(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.DeleteSpaceRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if err := s.store.Delete(payload.Name); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (s *Server) getQuarkfile(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.QuarkfileRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	data, err := s.store.Quarkfile(payload.Name)
	if err != nil {
		return nil, err
	}
	sp, err := s.store.Get(payload.Name)
	if err != nil {
		return nil, err
	}
	return clientcontract.QuarkfileResponse{
		Name:      payload.Name,
		Version:   sp.Version,
		Quarkfile: append([]byte(nil), data...),
		UpdatedAt: sp.UpdatedAt,
	}, nil
}

func (s *Server) doctor(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.DoctorRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	resp, err := s.store.Doctor(payload.Name)
	if err != nil {
		return nil, err
	}
	issues := make([]clientcontract.DoctorIssue, 0, len(resp.Issues))
	for _, issue := range resp.Issues {
		issues = append(issues, clientcontract.DoctorIssue{
			Severity: issue.Severity,
			Message:  issue.Message,
		})
	}
	return clientcontract.DoctorResponse{OK: resp.OK, Issues: issues}, nil
}

func (s *Server) createSession(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.CreateSessionRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.Sessions(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	sess, err := store.Create(sessions.Type(payload.Type), payload.Title)
	if err != nil {
		return nil, err
	}
	out := toContractSession(sess)
	s.events.Publish(event.Event{
		Space:   payload.SpaceID,
		Kind:    events.SessionCreated,
		Payload: events.SessionPayload(out.ID, string(out.Type), out.Title),
	})
	return out, nil
}

func (s *Server) listSessions(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.ListSessionsRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.Sessions(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	sessions := store.List()
	out := make([]clientcontract.SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, toContractSession(sess))
	}
	return clientcontract.ListSessionsResponse{Sessions: out}, nil
}

func (s *Server) getSession(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SessionRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.Sessions(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	sess, err := store.Get(payload.SessionID)
	if err != nil {
		return nil, err
	}
	return toContractSession(sess), nil
}

func (s *Server) deleteSession(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SessionRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.Sessions(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	if err := store.Delete(payload.SessionID); err != nil {
		return nil, err
	}
	s.events.Publish(event.Event{
		Space:   payload.SpaceID,
		Kind:    events.SessionDeleted,
		Payload: events.SessionPayload(payload.SessionID, "", ""),
	})
	return struct{}{}, nil
}

func (s *Server) getKB(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.KBRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.KB(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	value, err := store.Get(payload.Namespace, payload.Key)
	if err != nil {
		return nil, err
	}
	return clientcontract.KBValueResponse{Value: append([]byte(nil), value...)}, nil
}

func (s *Server) setKB(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.KBSetRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.KB(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	if err := store.Set(payload.Namespace, payload.Key, append([]byte(nil), payload.Value...)); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (s *Server) deleteKB(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.KBRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.KB(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	if err := store.Delete(payload.Namespace, payload.Key); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (s *Server) listKB(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.KBListRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	store, err := s.store.KB(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	keys, err := store.List(payload.Namespace)
	if err != nil {
		return nil, err
	}
	return clientcontract.KBListResponse{Keys: append([]string(nil), keys...)}, nil
}

func (s *Server) listPlugins(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.ListPluginsRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	var installed []pluginmanager.InstalledPlugin
	if payload.TypeFilter != "" {
		installed, err = mgr.ListByType(plugin.PluginType(payload.TypeFilter))
	} else {
		installed, err = mgr.List()
	}
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.PluginInfo, 0, len(installed))
	for _, item := range installed {
		out = append(out, toContractPlugin(item))
	}
	return clientcontract.ListPluginsResponse{Plugins: out}, nil
}

func (s *Server) getPlugin(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.PluginRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	item, err := mgr.Get(payload.Plugin)
	if err != nil {
		return nil, err
	}
	return toContractPlugin(item), nil
}

func (s *Server) installPlugin(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.InstallPluginRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	installed, err := mgr.Install(context.Background(), payload.Ref)
	if err != nil {
		return nil, err
	}
	return clientcontract.InstallPluginResponse{Plugin: toContractPlugin(*installed)}, nil
}

func (s *Server) uninstallPlugin(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.PluginRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	if err := mgr.Uninstall(payload.Plugin); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (s *Server) searchPlugins(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SearchPluginsRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	results, err := mgr.Search(payload.Query)
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.PluginSearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, clientcontract.PluginSearchResult{
			Name:        result.Name,
			Version:     result.Version,
			Type:        result.Type,
			Description: result.Description,
			Author:      result.Author,
		})
	}
	return clientcontract.SearchPluginsResponse{Results: out}, nil
}

func (s *Server) hubPluginInfo(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.PluginRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	info, err := mgr.GetHubInfo(payload.Plugin)
	if err != nil {
		return nil, err
	}
	return clientcontract.HubPluginInfo{
		Name:        info.Name,
		Version:     info.Version,
		Type:        info.Type,
		Description: info.Description,
		Author:      info.Author,
		License:     info.License,
		Repository:  info.Repository,
		Downloads:   info.Downloads,
		Versions:    append([]string(nil), info.Versions...),
	}, nil
}

func toContractSpace(sp *space.Space) clientcontract.SpaceInfo {
	if sp == nil {
		return clientcontract.SpaceInfo{}
	}
	return clientcontract.SpaceInfo{
		Name:       sp.Name,
		Version:    sp.Version,
		WorkingDir: sp.WorkingDir,
		CreatedAt:  sp.CreatedAt,
		UpdatedAt:  sp.UpdatedAt,
	}
}

func toContractPlugin(item pluginmanager.InstalledPlugin) clientcontract.PluginInfo {
	return clientcontract.PluginInfo{
		Name:        item.Manifest.Name,
		Version:     item.Manifest.Version,
		Type:        string(item.Manifest.Type),
		Mode:        string(item.Manifest.Mode),
		Description: item.Manifest.Description,
	}
}

func toContractSession(sess *sessions.Session) clientcontract.SessionInfo {
	if sess == nil {
		return clientcontract.SessionInfo{}
	}
	return clientcontract.SessionInfo{
		ID:        sess.ID,
		Type:      clientcontract.SessionType(sess.Type),
		Title:     sess.Title,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	}
}
