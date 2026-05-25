package natshub

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type CredentialRole string

const (
	RoleSupervisor    CredentialRole = "supervisor"
	RoleUser          CredentialRole = "user"
	RoleRuntime       CredentialRole = "runtime"
	RoleSession       CredentialRole = "session"
	RoleAgent         CredentialRole = "agent"
	RoleService       CredentialRole = "service"
	RoleObservability CredentialRole = "observability"
)

var ErrProvisioningUnavailable = errors.New("nats account provisioning is unavailable")

type Credential struct {
	Username    string
	Password    string
	Account     string
	Role        CredentialRole
	SpaceID     string
	SessionID   string
	AgentID     string
	Permissions PermissionConfig
}

type SpaceCredentials struct {
	SpaceID       string
	Account       string
	Runtime       Credential
	Service       Credential
	Observability Credential
}

func (h *Hub) ControlCredential() (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.controlCredentialLocked()
}

func (h *Hub) controlCredentialLocked() (Credential, error) {
	for _, account := range h.cfg.Accounts {
		if account.Name != ControlAccountName {
			continue
		}
		if len(account.Users) == 0 {
			return Credential{}, errors.New("nats control account has no users")
		}
		user := cloneUserConfig(account.Users[0])
		return Credential{
			Username:    user.Name,
			Password:    user.Password,
			Account:     ControlAccountName,
			Role:        RoleSupervisor,
			Permissions: clonePermissions(user.Permissions),
		}, nil
	}
	return Credential{}, errors.New("nats control account is not configured")
}

func spaceCredential(spaceID, account string, role CredentialRole, permissions PermissionConfig) (Credential, error) {
	username := credentialUsername(spaceID, string(role))
	password, err := randomPassword()
	if err != nil {
		return Credential{}, err
	}
	return Credential{
		Username:    username,
		Password:    password,
		Account:     account,
		Role:        role,
		SpaceID:     spaceID,
		Permissions: clonePermissions(permissions),
	}, nil
}

func sessionCredential(spaceID, account, sessionID string) (Credential, error) {
	if strings.TrimSpace(sessionID) == "" {
		return Credential{}, errors.New("session id is required")
	}
	credential, err := spaceCredential(spaceID, account, RoleSession, SessionPermissions(sessionID))
	if err != nil {
		return Credential{}, err
	}
	credential.SessionID = sessionID
	credential.Username = credentialUsername(spaceID, "session_"+stableToken(sessionID))
	return credential, nil
}

func userCredential(spaceID, account string) (Credential, error) {
	return spaceCredential(spaceID, account, RoleUser, UserPermissions())
}

func agentCredential(spaceID, account, agentID string) (Credential, error) {
	if strings.TrimSpace(agentID) == "" {
		return Credential{}, errors.New("agent id is required")
	}
	credential, err := spaceCredential(spaceID, account, RoleAgent, AgentPermissions(agentID))
	if err != nil {
		return Credential{}, err
	}
	credential.AgentID = agentID
	credential.Username = credentialUsername(spaceID, "agent_"+stableToken(agentID))
	return credential, nil
}

func serviceCredential(service, account string, routes []ServiceFunctionRoute) (Credential, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		return Credential{}, errors.New("service name is required")
	}
	credential, err := spaceCredential(ControlAccountName, account, RoleService, ServiceHostPermissions(service, routes))
	if err != nil {
		return Credential{}, err
	}
	credential.SpaceID = ""
	credential.Username = credentialUsername(ControlAccountName, "service_"+stableToken(service))
	return credential, nil
}

func credentialUsername(spaceID, role string) string {
	return "quark_" + strings.ToLower(stableToken(spaceID)) + "_" + strings.ToLower(stableToken(role))
}

func issuedCredentialKey(role CredentialRole, owner, scope string) string {
	return string(role) + ":" + stableToken(owner) + ":" + stableToken(scope)
}

func cloneCredential(in Credential) Credential {
	out := in
	out.Permissions = clonePermissions(in.Permissions)
	return out
}

func cloneSpaceCredentials(in SpaceCredentials) SpaceCredentials {
	return SpaceCredentials{
		SpaceID:       in.SpaceID,
		Account:       in.Account,
		Runtime:       cloneCredential(in.Runtime),
		Service:       cloneCredential(in.Service),
		Observability: cloneCredential(in.Observability),
	}
}

func userConfigFromCredential(credential Credential) UserConfig {
	return UserConfig{
		Name:        credential.Username,
		Password:    credential.Password,
		Permissions: clonePermissions(credential.Permissions),
	}
}

func appendUserToAccount(accounts []AccountConfig, accountName string, user UserConfig) []AccountConfig {
	out := cloneAccounts(accounts)
	for i := range out {
		if out[i].Name != accountName {
			continue
		}
		for j := range out[i].Users {
			if out[i].Users[j].Name == user.Name {
				out[i].Users[j] = cloneUserConfig(user)
				return out
			}
		}
		out[i].Users = append(out[i].Users, cloneUserConfig(user))
		return out
	}
	return append(out, AccountConfig{Name: accountName, Users: []UserConfig{cloneUserConfig(user)}})
}

func upsertAccountUsers(accounts []AccountConfig, account AccountConfig) []AccountConfig {
	out := cloneAccounts(accounts)
	for i := range out {
		if out[i].Name != account.Name {
			continue
		}
		for _, user := range account.Users {
			out = appendUserToAccount(out, account.Name, user)
		}
		return out
	}
	out = append(out, cloneAccountConfig(account))
	return out
}

func cloneAccounts(accounts []AccountConfig) []AccountConfig {
	out := make([]AccountConfig, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, cloneAccountConfig(account))
	}
	return out
}

func cloneAccountConfig(account AccountConfig) AccountConfig {
	out := account
	out.Users = make([]UserConfig, 0, len(account.Users))
	for _, user := range account.Users {
		out.Users = append(out.Users, cloneUserConfig(user))
	}
	return out
}

func cloneUserConfig(user UserConfig) UserConfig {
	out := user
	out.Permissions = clonePermissions(user.Permissions)
	return out
}

func randomPassword() (string, error) {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate nats credential password: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
