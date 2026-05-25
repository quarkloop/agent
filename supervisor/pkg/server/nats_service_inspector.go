package server

import (
	"context"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

type natsServiceInspector struct {
	server *Server
}

func (i natsServiceInspector) InspectServices(ctx context.Context, spaceID string) ([]clientcontract.ServiceInfo, error) {
	return i.server.inspectServices(ctx, spaceID)
}
