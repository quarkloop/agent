package startup

import (
	"os"
	"strings"
)

const (
	EnvAgentProfile  = "QUARK_AGENT_PROFILE"
	EnvModelProvider = "QUARK_MODEL_PROVIDER"
	EnvModelName     = "QUARK_MODEL_NAME"
	EnvPrimarySpace  = "QUARK_SPACE"
	EnvRuntimeSpaces = "QUARK_SPACES"
)

type Environment struct {
	AgentProfile  string
	ModelProvider string
	ModelName     string
	PrimarySpace  string
	RuntimeSpaces string
}

func EnvironmentFromOS() Environment {
	return Environment{
		AgentProfile:  strings.TrimSpace(os.Getenv(EnvAgentProfile)),
		ModelProvider: strings.TrimSpace(os.Getenv(EnvModelProvider)),
		ModelName:     strings.TrimSpace(os.Getenv(EnvModelName)),
		PrimarySpace:  strings.TrimSpace(os.Getenv(EnvPrimarySpace)),
		RuntimeSpaces: strings.TrimSpace(os.Getenv(EnvRuntimeSpaces)),
	}
}

func (e Environment) Spaces() []string {
	values := make([]string, 0)
	add := func(value string) {
		for _, part := range strings.Split(value, ",") {
			space := strings.TrimSpace(part)
			if space != "" {
				values = append(values, space)
			}
		}
	}
	add(e.RuntimeSpaces)
	if len(values) == 0 {
		add(e.PrimarySpace)
	}
	seen := make(map[string]struct{}, len(values))
	out := values[:0]
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func SpacesFromEnv() []string {
	return EnvironmentFromOS().Spaces()
}

func EnsurePrimarySpaceEnv(spaces []string) error {
	if os.Getenv(EnvPrimarySpace) != "" || len(spaces) == 0 {
		return nil
	}
	return os.Setenv(EnvPrimarySpace, spaces[0])
}

func SpaceToken(value string) string {
	token := strings.TrimSpace(value)
	token = strings.ToLower(token)
	token = strings.ReplaceAll(token, "/", "_")
	token = strings.ReplaceAll(token, ".", "_")
	token = strings.ReplaceAll(token, "-", "_")
	if token == "" {
		return "space"
	}
	return token
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
