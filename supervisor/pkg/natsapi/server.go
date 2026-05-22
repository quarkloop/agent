package natsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
	event "github.com/quarkloop/pkg/event"
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
		clientcontract.SubjectSpaceCreate:   s.createSpace,
		clientcontract.SubjectSpaceList:     s.listSpaces,
		clientcontract.SubjectSpaceGet:      s.getSpace,
		clientcontract.SubjectSpaceUpdate:   s.updateSpace,
		clientcontract.SubjectSessionCreate: s.createSession,
		clientcontract.SubjectSessionList:   s.listSessions,
		clientcontract.SubjectSessionGet:    s.getSession,
		clientcontract.SubjectSessionDelete: s.deleteSession,
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
