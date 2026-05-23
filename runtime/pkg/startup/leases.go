package startup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/quarkloop/runtime/pkg/spacelease"
)

func ClaimRuntimeSpaces(ctx context.Context, spaces []string) (*spacelease.Manager, []*spacelease.Lease, error) {
	if len(spaces) == 0 {
		return nil, nil, nil
	}
	cfg := spacelease.ConfigFromEnv()
	if strings.TrimSpace(cfg.URL) == "" {
		slog.Warn("runtime space leases disabled because QUARK_NATS_URL is empty", "spaces", spaces)
		return nil, nil, nil
	}
	manager, err := spacelease.New(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime space lease manager: %w", err)
	}
	leases := make([]*spacelease.Lease, 0, len(spaces))
	for _, spaceID := range spaces {
		lease, err := manager.Claim(ctx, spaceID)
		if err != nil {
			ReleaseRuntimeSpaces(context.Background(), leases, manager)
			return nil, nil, err
		}
		lease.StartRenewal(ctx)
		leases = append(leases, lease)
		slog.Info("runtime space lease claimed", "space", spaceID, "runtime", lease.RuntimeID)
	}
	return manager, leases, nil
}

func ReleaseRuntimeSpaces(ctx context.Context, leases []*spacelease.Lease, manager *spacelease.Manager) {
	for _, lease := range leases {
		if err := lease.Release(ctx); err != nil {
			slog.Warn("release runtime space lease failed", "space", lease.SpaceID, "error", err)
		}
	}
	if manager != nil {
		manager.Close()
	}
}
