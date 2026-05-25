package natsapi

import (
	"context"
	"strings"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func (s *Server) getAuditRecord(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.AuditGetRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.SpaceID) == "" || strings.TrimSpace(payload.ReferenceID) == "" {
		return nil, boundary.New(boundary.Supervisor, boundary.InvalidArgument, clientcontract.SubjectAuditGet, "space_id and reference_id are required")
	}
	if s.auditReader == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectAuditGet, "audit reader is not configured")
	}
	record, err := s.auditReader.GetAuditRecord(context.Background(), payload.SpaceID, payload.ReferenceID)
	if err != nil {
		return nil, err
	}
	return toContractAuditRecord(record), nil
}

func (s *Server) listAuditRecords(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.AuditListRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		return nil, boundary.New(boundary.Supervisor, boundary.InvalidArgument, clientcontract.SubjectAuditList, "space_id is required")
	}
	if s.auditReader == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectAuditList, "audit reader is not configured")
	}
	page, err := s.auditReader.ListAuditRecords(context.Background(), natshub.AuditFilter{
		SpaceID: payload.SpaceID, SessionID: payload.SessionID, RunID: payload.RunID,
		Service: payload.Service, Function: payload.Function, Limit: payload.Limit, Cursor: payload.Cursor,
	})
	if err != nil {
		return nil, err
	}
	return toContractAuditPage(page), nil
}

func (s *Server) auditRetention(clientcontract.RequestEnvelope) (any, error) {
	if s.auditReader == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectAuditRetention, "audit reader is not configured")
	}
	retention := s.auditReader.AuditRetention()
	return clientcontract.AuditRetentionResponse{
		MaxAgeSeconds: int64(retention.MaxAge.Seconds()),
		MaxMessages:   retention.MaxMessages,
	}, nil
}
