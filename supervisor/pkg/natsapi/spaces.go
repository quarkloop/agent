package natsapi

import (
	"fmt"

	"github.com/quarkloop/pkg/boundary"
	event "github.com/quarkloop/pkg/event"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/events"
)

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
	if err := s.publishCatalogEvent(payload.Name, "space_created"); err != nil {
		return nil, err
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
	if err := s.publishCatalogEvent(payload.Name, "quarkfile_updated"); err != nil {
		return nil, err
	}
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
	if err := s.publishCatalogEvent(payload.Name, "space_deleted"); err != nil {
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

func (s *Server) spaceCredential(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SpaceCredentialRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if s.credentialIssuer == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectSpaceCredential, "space credential issuer is not configured")
	}
	if _, err := s.store.Get(payload.SpaceID); err != nil {
		return nil, err
	}
	credential, err := s.credentialIssuer.IssueUserCredential(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	return clientcontract.SpaceCredentialResponse{
		Credential: toContractCredential(s.url, credential),
	}, nil
}

func (s *Server) runtimeCredential(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SpaceCredentialRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if s.credentialIssuer == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectRuntimeCredential, "runtime credential issuer is not configured")
	}
	if _, err := s.store.Get(payload.SpaceID); err != nil {
		return nil, err
	}
	credential, err := s.credentialIssuer.IssueRuntimeCredential(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	return clientcontract.SpaceCredentialResponse{
		Credential: toContractCredential(s.url, credential),
	}, nil
}
