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
	sess, err := s.sessions.Create(payload.SpaceID, sessions.Type(payload.Type), payload.Title)
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
	items, err := s.sessions.List(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.SessionInfo, 0, len(items))
	for _, sess := range items {
		out = append(out, toContractSession(sess))
	}
	return clientcontract.ListSessionsResponse{Sessions: out}, nil
}

func (s *Server) getSession(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SessionRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	sess, err := s.sessions.Get(payload.SpaceID, payload.SessionID)
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
	if _, err := s.sessions.Get(payload.SpaceID, payload.SessionID); err != nil {
		return nil, err
	}
	credential, err := s.credentialIssuer.IssueSessionCredential(payload.SpaceID, payload.SessionID)
	if err != nil {
		return nil, err
	}
	return clientcontract.SessionCredentialResponse{
		Credential: toContractCredential(credential),
	}, nil
}

func (s *Server) deleteSession(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SessionRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if err := s.sessions.Delete(payload.SpaceID, payload.SessionID); err != nil {
		return nil, err
	}
	s.events.Publish(event.Event{
		Space:   payload.SpaceID,
		Kind:    events.SessionDeleted,
		Payload: events.SessionPayload(payload.SessionID, "", ""),
	})
	return struct{}{}, nil
}
