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

func (c *Client) ServiceLogs(ctx context.Context, space, service string) (api.ServiceLogsResponse, error) {
	var out api.ServiceLogsResponse
	err := c.do(ctx, http.MethodGet, c.route.SpaceServiceLogs(space, service), nil, &out)
	return out, err
}

func (c *Client) StartService(ctx context.Context, space, service string) (api.ServiceLifecycleResponse, error) {
	var out api.ServiceLifecycleResponse
	err := c.do(ctx, http.MethodPost, c.route.SpaceServiceStart(space, service), nil, &out)
	return out, err
}

func (c *Client) StopService(ctx context.Context, space, service string) (api.ServiceLifecycleResponse, error) {
	var out api.ServiceLifecycleResponse
	err := c.do(ctx, http.MethodPost, c.route.SpaceServiceStop(space, service), nil, &out)
	return out, err
}

func (c *Client) RestartService(ctx context.Context, space, service string) (api.ServiceRestartResponse, error) {
	var out api.ServiceRestartResponse
	err := c.do(ctx, http.MethodPost, c.route.SpaceServiceRestart(space, service), nil, &out)
	return out, err
}

func (c *Client) ServiceDoctor(ctx context.Context, space string) (api.ServiceDoctorResponse, error) {
	var out api.ServiceDoctorResponse
	err := c.do(ctx, http.MethodPost, c.route.SpaceServiceDoctor(space), nil, &out)
	return out, err
}
