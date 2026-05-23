package startup

import (
	"fmt"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/runtime/pkg/spaceauth"
)

type SpaceConfig struct {
	SpaceID    string
	Credential clientcontract.NATSCredential
}

func SpaceConfigsFromEnv(spaces []string) ([]SpaceConfig, error) {
	if len(spaces) == 0 {
		return nil, nil
	}
	resolver, err := spaceauth.ResolverFromEnv()
	if err != nil {
		return nil, err
	}
	if len(spaces) > 1 && !resolver.HasExplicitCredentials() {
		return nil, fmt.Errorf("%s is required when one runtime serves multiple spaces", spaceauth.EnvRuntimeSpaceCredentials)
	}
	out := make([]SpaceConfig, 0, len(spaces))
	for _, spaceID := range spaces {
		credential, err := resolver.Resolve(spaceID)
		if err != nil {
			return nil, err
		}
		out = append(out, SpaceConfig{SpaceID: spaceID, Credential: credential})
	}
	return out, nil
}
