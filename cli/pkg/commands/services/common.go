package servicescmd

import (
	"fmt"

	spacemodel "github.com/quarkloop/pkg/space"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

func currentSpaceName() (string, error) {
	name, err := spacemodel.CurrentName()
	if err != nil {
		return "", err
	}
	return name, nil
}

func newSupervisorClient() *supclient.Client {
	return supclient.New()
}

func serviceCommandError(action string, err error) error {
	return fmt.Errorf("%s failed: %w", action, err)
}
