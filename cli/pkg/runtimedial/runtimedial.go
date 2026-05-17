// Package runtimedial provides helpers for connecting the CLI to the runtime
// process of the current space. All lookups go through the supervisor —
// the CLI never reads the filesystem to discover a running runtime.
package runtimedial

import (
	"context"
	"fmt"

	spacemodel "github.com/quarkloop/pkg/space"
	rtclient "github.com/quarkloop/runtime/pkg/client"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

// Current resolves the running runtime for the current working directory's
// space and returns an HTTP client pointed at it.
//
// The supervisor is the source of truth; if no runtime is running for the
// current space, an error is returned.
func Current(ctx context.Context) (*rtclient.Client, supclient.RuntimeInfo, error) {
	return CurrentWithTransportOptions(ctx)
}

// CurrentWithTransportOptions resolves the running runtime and constructs a
// runtime client with explicit HTTP transport options.
func CurrentWithTransportOptions(ctx context.Context, opts ...rtclient.TransportOption) (*rtclient.Client, supclient.RuntimeInfo, error) {
	name, err := spacemodel.CurrentName()
	if err != nil {
		return nil, supclient.RuntimeInfo{}, err
	}
	sup := supclient.New()
	rt, err := sup.RuntimeBySpace(ctx, name)
	if err != nil {
		if supclient.IsNotFound(err) {
			return nil, supclient.RuntimeInfo{}, fmt.Errorf("no runtime running for space %q — start it with 'quark run'", name)
		}
		return nil, supclient.RuntimeInfo{}, err
	}
	if rt.Status != supclient.RuntimeRunning {
		return nil, supclient.RuntimeInfo{}, fmt.Errorf("runtime for space %q is %s, not running", name, rt.Status)
	}
	transport := rtclient.NewTransport(rt.URL(), opts...)
	return rtclient.New(rt.URL(), rtclient.WithTransport(transport)), rt, nil
}
