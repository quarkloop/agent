package natshub

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

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
