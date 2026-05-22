package servicescmd

import (
	"context"
	"fmt"

	"github.com/quarkloop/cli/pkg/natsclient"
	spacemodel "github.com/quarkloop/pkg/space"
)

func currentSpaceName() (string, error) {
	name, err := spacemodel.CurrentName()
	if err != nil {
		return "", err
	}
	return name, nil
}

func newControlClient(ctx context.Context) (*natsclient.Client, error) {
	return natsclient.ConnectFromEnv(ctx)
}

func serviceCommandError(action string, err error) error {
	return fmt.Errorf("%s failed: %w", action, err)
}
