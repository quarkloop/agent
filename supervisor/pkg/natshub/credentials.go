package natshub

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

type CredentialRole string

const (
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

type credentialRecord struct {
	password string
	user     natsserver.User
}

type credentialRegistry struct {
	mu    sync.RWMutex
	users map[string]credentialRecord
}

func newCredentialRegistry() *credentialRegistry {
	return &credentialRegistry{users: make(map[string]credentialRecord)}
}

func (r *credentialRegistry) add(account *natsserver.Account, credential Credential) error {
	if account == nil {
		return fmt.Errorf("nats account %q is not registered", credential.Account)
	}
	if strings.TrimSpace(credential.Username) == "" {
		return errors.New("nats credential username is required")
	}
	if strings.TrimSpace(credential.Password) == "" {
		return errors.New("nats credential password is required")
	}
	user := natsserver.User{
		Username:    credential.Username,
		Password:    credential.Password,
		Account:     account,
		Permissions: toServerPermissions(credential.Permissions),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.users[credential.Username]; exists {
		return fmt.Errorf("nats credential %q already exists", credential.Username)
	}
	r.users[credential.Username] = credentialRecord{password: credential.Password, user: user}
	return nil
}

func (r *credentialRegistry) user(username, password string) (*natsserver.User, bool) {
	r.mu.RLock()
	record, ok := r.users[username]
	r.mu.RUnlock()
	if !ok || record.password != password {
		return nil, false
	}
	user := record.user
	user.Password = password
	return &user, true
}

func (r *credentialRegistry) rebindAccounts(accounts map[string]*natsserver.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for username, record := range r.users {
		if record.user.Account == nil {
			continue
		}
		accountName := record.user.Account.Name
		account, ok := accounts[accountName]
		if !ok {
			return fmt.Errorf("nats credential %q references missing account %q", username, accountName)
		}
		record.user.Account = account
		r.users[username] = record
	}
	return nil
}

type registryAuthenticator struct {
	registry *credentialRegistry
}

func (a registryAuthenticator) Check(c natsserver.ClientAuthentication) bool {
	if a.registry == nil || c.Kind() != natsserver.CLIENT {
		return false
	}
	opts := c.GetOpts()
	user, ok := a.registry.user(opts.Username, opts.Password)
	if !ok {
		return false
	}
	c.RegisterUser(user)
	return true
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
	permissions := PermissionConfig{
		PublishAllow: []string{"_INBOX.>", "_R_.>"},
	}
	for _, route := range routes {
		permissions.SubscribeAllow = append(permissions.SubscribeAllow, route.ExportSubject)
	}
	credential, err := spaceCredential(ControlAccountName, account, RoleService, permissions)
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
