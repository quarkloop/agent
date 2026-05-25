package natsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (s *Server) runtimeCatalog(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.RuntimeCatalogRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		return nil, boundary.New(boundary.Supervisor, boundary.InvalidArgument, clientcontract.SubjectCatalogRuntimeGet, "space_id is required")
	}
	if s.catalogResolver == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectCatalogRuntimeGet, "runtime catalog resolver is not configured")
	}
	return s.catalogResolver.RuntimeCatalogSnapshot(context.Background(), payload.SpaceID)
}

func (s *Server) publishCatalogEvent(spaceID, reason string) error {
	if s == nil || s.client == nil {
		return nil
	}
	event := clientcontract.RuntimeCatalogEvent{
		SpaceID:     spaceID,
		Reason:      reason,
		GeneratedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal catalog event: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.client.Publish(ctx, clientcontract.SubjectCatalogRuntimeEvents, data, nil); err != nil {
		return fmt.Errorf("publish catalog event: %w", err)
	}
	return nil
}
