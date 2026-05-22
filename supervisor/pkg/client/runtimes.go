package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/quarkloop/supervisor/pkg/api"
)

// RuntimeStatus is the lifecycle state of a running runtime.
type RuntimeStatus = api.RuntimeStatus

// Runtime status constants.
const (
	RuntimeStarting RuntimeStatus = api.RuntimeStarting
	RuntimeRunning  RuntimeStatus = api.RuntimeRunning
	RuntimeStopping RuntimeStatus = api.RuntimeStopping
	RuntimeStopped  RuntimeStatus = api.RuntimeStopped
)

// RuntimeInfo describes runtime information observed by the supervisor API.
type RuntimeInfo = api.RuntimeInfo

// ListRuntimes returns every runtime tracked by the supervisor.
func (c *Client) ListRuntimes(ctx context.Context) ([]api.RuntimeInfo, error) {
	var out []api.RuntimeInfo
	if err := c.do(ctx, http.MethodGet, c.route.Agents(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetRuntime returns the current state of a single runtime by ID.
func (c *Client) GetRuntime(ctx context.Context, id string) (api.RuntimeInfo, error) {
	var out api.RuntimeInfo
	err := c.do(ctx, http.MethodGet, c.route.Agent(id), nil, &out)
	return out, err
}

// RuntimeBySpace returns the running runtime for the given space. Returns an
// HTTPError with status 404 when no runtime is running for the space.
func (c *Client) RuntimeBySpace(ctx context.Context, space string) (api.RuntimeInfo, error) {
	runtimes, err := c.ListRuntimes(ctx)
	if err != nil {
		return api.RuntimeInfo{}, err
	}
	for _, r := range runtimes {
		if r.Space == space {
			return r, nil
		}
	}
	return api.RuntimeInfo{}, &HTTPError{
		Method:     http.MethodGet,
		Path:       c.route.Agents(),
		StatusCode: http.StatusNotFound,
		Body:       fmt.Sprintf("no runtime running for space %q", space),
	}
}
