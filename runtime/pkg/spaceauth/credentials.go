package spaceauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

const (
	EnvNATSURL                 = "QUARK_NATS_URL"
	EnvNATSUser                = "QUARK_NATS_USER"
	EnvNATSPassword            = "QUARK_NATS_PASSWORD"
	EnvRuntimeSpaceCredentials = "QUARK_RUNTIME_SPACE_CREDENTIALS"
)

type Resolver struct {
	defaultCredential clientcontract.NATSCredential
	bySpace           map[string]clientcontract.NATSCredential
}

func ResolverFromEnv() (Resolver, error) {
	defaultCredential := clientcontract.NATSCredential{
		URL:      strings.TrimSpace(os.Getenv(EnvNATSURL)),
		Username: strings.TrimSpace(os.Getenv(EnvNATSUser)),
		Password: os.Getenv(EnvNATSPassword),
	}
	resolver := Resolver{
		defaultCredential: defaultCredential,
		bySpace:           make(map[string]clientcontract.NATSCredential),
	}
	raw := strings.TrimSpace(os.Getenv(EnvRuntimeSpaceCredentials))
	if raw == "" {
		return resolver, nil
	}
	credentials, err := parseSpaceCredentials([]byte(raw))
	if err != nil {
		return Resolver{}, err
	}
	for _, credential := range credentials {
		spaceID := strings.TrimSpace(credential.SpaceID)
		if spaceID == "" {
			return Resolver{}, errors.New("runtime space credential is missing space_id")
		}
		if strings.TrimSpace(credential.URL) == "" {
			credential.URL = defaultCredential.URL
		}
		resolver.bySpace[spaceID] = cloneCredential(credential)
	}
	return resolver, nil
}

func (r Resolver) Resolve(spaceID string) (clientcontract.NATSCredential, error) {
	spaceID = strings.TrimSpace(spaceID)
	if spaceID == "" {
		return clientcontract.NATSCredential{}, errors.New("space_id is required")
	}
	if credential, ok := r.bySpace[spaceID]; ok {
		return normalizedCredential(spaceID, credential)
	}
	credential := cloneCredential(r.defaultCredential)
	credential.SpaceID = spaceID
	return normalizedCredential(spaceID, credential)
}

func (r Resolver) HasExplicitCredentials() bool {
	return len(r.bySpace) > 0
}

func parseSpaceCredentials(data []byte) ([]clientcontract.NATSCredential, error) {
	var list []clientcontract.NATSCredential
	if err := json.Unmarshal(data, &list); err == nil {
		return cloneCredentials(list), nil
	}
	var bySpace map[string]clientcontract.NATSCredential
	if err := json.Unmarshal(data, &bySpace); err != nil {
		return nil, fmt.Errorf("parse %s: %w", EnvRuntimeSpaceCredentials, err)
	}
	list = make([]clientcontract.NATSCredential, 0, len(bySpace))
	for spaceID, credential := range bySpace {
		if strings.TrimSpace(credential.SpaceID) == "" {
			credential.SpaceID = spaceID
		}
		list = append(list, credential)
	}
	return cloneCredentials(list), nil
}

func normalizedCredential(spaceID string, credential clientcontract.NATSCredential) (clientcontract.NATSCredential, error) {
	credential = cloneCredential(credential)
	credential.SpaceID = spaceID
	credential.URL = strings.TrimSpace(credential.URL)
	credential.Username = strings.TrimSpace(credential.Username)
	if credential.URL == "" {
		return clientcontract.NATSCredential{}, fmt.Errorf("nats url is required for space %q", spaceID)
	}
	if credential.Username == "" {
		return clientcontract.NATSCredential{}, fmt.Errorf("nats username is required for space %q", spaceID)
	}
	return credential, nil
}

func cloneCredentials(in []clientcontract.NATSCredential) []clientcontract.NATSCredential {
	out := make([]clientcontract.NATSCredential, len(in))
	for i, credential := range in {
		out[i] = cloneCredential(credential)
	}
	return out
}

func cloneCredential(in clientcontract.NATSCredential) clientcontract.NATSCredential {
	return clientcontract.NATSCredential{
		URL:       in.URL,
		Username:  in.Username,
		Password:  in.Password,
		Account:   in.Account,
		Role:      in.Role,
		SpaceID:   in.SpaceID,
		SessionID: in.SessionID,
		AgentID:   in.AgentID,
	}
}
