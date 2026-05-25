package client

import (
	"context"
	"net/http"

	"github.com/quarkloop/supervisor/pkg/api"
)

// ListSpaces returns every space known to the supervisor.
func (c *Client) ListSpaces(ctx context.Context) ([]api.SpaceInfo, error) {
	var out []api.SpaceInfo
	if err := c.do(ctx, http.MethodGet, c.route.Spaces(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSpace registers a new space using an authoritative configuration
// serialized for the Space service.
func (c *Client) CreateSpace(ctx context.Context, config []byte) (api.SpaceInfo, error) {
	var out api.SpaceInfo
	err := c.do(ctx, http.MethodPost, c.route.Spaces(), api.CreateSpaceRequest{
		Config: config,
	}, &out)
	return out, err
}

// GetSpace returns metadata for a single space.
func (c *Client) GetSpace(ctx context.Context, name string) (api.SpaceInfo, error) {
	var out api.SpaceInfo
	err := c.do(ctx, http.MethodGet, c.route.Space(name), nil, &out)
	return out, err
}

// DeleteSpace permanently removes a space and all of its data.
func (c *Client) DeleteSpace(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, c.route.Space(name), nil, nil)
}

// SpaceConfig returns the authoritative stored configuration for the space.
func (c *Client) SpaceConfig(ctx context.Context, name string) (api.SpaceConfigResponse, error) {
	var out api.SpaceConfigResponse
	err := c.do(ctx, http.MethodGet, c.route.SpaceConfig(name), nil, &out)
	return out, err
}

// UpdateSpaceConfig replaces the authoritative configuration for the space.
func (c *Client) UpdateSpaceConfig(ctx context.Context, name string, config []byte) (api.SpaceInfo, error) {
	var out api.SpaceInfo
	err := c.do(ctx, http.MethodPut, c.route.SpaceConfig(name),
		api.UpdateSpaceConfigRequest{Config: config}, &out)
	return out, err
}

// Doctor runs supervisor-side health checks against the space.
func (c *Client) Doctor(ctx context.Context, name string) (api.DoctorResponse, error) {
	var out api.DoctorResponse
	err := c.do(ctx, http.MethodPost, c.route.SpaceDoctor(name), nil, &out)
	return out, err
}
