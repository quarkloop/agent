// Package runtimeconnect resolves NATS credentials for runtime-facing CLI calls.
package runtimeconnect

import (
	"context"
	"fmt"

	"github.com/quarkloop/cli/pkg/natsclient"
	spacemodel "github.com/quarkloop/pkg/space"
)

type SpaceClient struct {
	SpaceID string
	Client  *natsclient.Client
}

func CurrentSpaceClient(ctx context.Context) (SpaceClient, error) {
	spaceID, err := spacemodel.CurrentName()
	if err != nil {
		return SpaceClient{}, err
	}
	control, err := natsclient.ConnectFromEnv(ctx)
	if err != nil {
		return SpaceClient{}, err
	}
	defer control.Close()
	credential, err := control.IssueSpaceCredential(ctx, spaceID)
	if err != nil {
		return SpaceClient{}, fmt.Errorf("issue space credential: %w", err)
	}
	client, err := natsclient.ConnectWithCredential(ctx, credential)
	if err != nil {
		return SpaceClient{}, err
	}
	return SpaceClient{SpaceID: spaceID, Client: client}, nil
}
