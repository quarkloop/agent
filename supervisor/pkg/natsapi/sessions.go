package natsapi

import (
	"github.com/quarkloop/pkg/boundary"
	event "github.com/quarkloop/pkg/event"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/events"
	"github.com/quarkloop/supervisor/pkg/sessions"
)

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

func (s *Server) sessionCredential(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SessionCredentialRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if s.credentialIssuer == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectSessionCredential, "session credential issuer is not configured")
	}
	store, err := s.store.Sessions(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	if _, err := store.Get(payload.SessionID); err != nil {
		return nil, err
	}
	credential, err := s.credentialIssuer.IssueSessionCredential(payload.SpaceID, payload.SessionID)
	if err != nil {
		return nil, err
	}
	return clientcontract.SessionCredentialResponse{
		Credential: toContractCredential(s.url, credential),
	}, nil
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
