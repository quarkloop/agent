package servicescmd

import (
	"context"
	"fmt"

	"github.com/quarkloop/cli/pkg/natsclient"
	"github.com/quarkloop/cli/pkg/spacecontext"
	"github.com/spf13/cobra"
)

func currentSpaceName(cmd *cobra.Command) (string, error) {
	name, err := spacecontext.FromCommand(cmd)
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
