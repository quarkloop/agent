package client

import (
	"context"
	"net/http"

	"github.com/quarkloop/supervisor/pkg/api"
)

func (c *Client) ListServices(ctx context.Context, space string) ([]api.ServiceInfo, error) {
	var out api.ListServicesResponse
	err := c.do(ctx, http.MethodGet, c.route.SpaceServices(space), nil, &out)
	return out.Services, err
}

func (c *Client) InspectService(ctx context.Context, space, service string) (api.ServiceInfo, error) {
	var out api.ServiceInfo
	err := c.do(ctx, http.MethodGet, c.route.SpaceService(space, service), nil, &out)
	return out, err
}

func (c *Client) ServiceDoctor(ctx context.Context, space string) (api.ServiceDoctorResponse, error) {
	var out api.ServiceDoctorResponse
	err := c.do(ctx, http.MethodPost, c.route.SpaceServiceDoctor(space), nil, &out)
	return out, err
}
