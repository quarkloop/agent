package natshub

import (
	"errors"
	"fmt"
	"strings"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func validateAccounts(systemAccount string, accounts []AccountConfig) error {
	names := make(map[string]struct{}, len(accounts))
	hasSystem := false
	users := map[string]struct{}{}
	for _, account := range accounts {
		name := strings.TrimSpace(account.Name)
		if name == "" {
			return errors.New("nats account name is required")
		}
		if _, ok := names[name]; ok {
			return fmt.Errorf("duplicate nats account %q", name)
		}
		names[name] = struct{}{}
		if name == systemAccount {
			hasSystem = true
		}
		for _, user := range account.Users {
			userName := strings.TrimSpace(user.Name)
			if userName == "" {
				return fmt.Errorf("nats account %q has a user without a name", name)
			}
			if strings.TrimSpace(user.Password) == "" {
				return fmt.Errorf("nats user %q in account %q requires a password", userName, name)
			}
			if _, ok := users[userName]; ok {
				return fmt.Errorf("duplicate nats user %q", userName)
			}
			users[userName] = struct{}{}
		}
	}
	if !hasSystem {
		return fmt.Errorf("nats system account %q is not declared", systemAccount)
	}
	return nil
}

func buildAccounts(configs []AccountConfig) ([]*natsserver.Account, map[string]*natsserver.Account, *credentialRegistry, error) {
	accounts := make([]*natsserver.Account, 0, len(configs))
	accountMap := make(map[string]*natsserver.Account, len(configs))
	credentials := newCredentialRegistry()
	for _, config := range configs {
		account := natsserver.NewAccount(strings.TrimSpace(config.Name))
		accounts = append(accounts, account)
		accountMap[account.Name] = account
		for _, user := range config.Users {
			credential := Credential{
				Username:    strings.TrimSpace(user.Name),
				Password:    user.Password,
				Account:     account.Name,
				Permissions: clonePermissions(user.Permissions),
			}
			if err := credentials.add(account, credential); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	return accounts, accountMap, credentials, nil
}

func (h *Hub) accountLocked(accountName string) (*natsserver.Account, error) {
	if account, ok := h.accounts[accountName]; ok {
		return account, nil
	}
	if h.server == nil {
		return nil, errors.New("embedded nats server is not started")
	}
	account, err := h.server.RegisterAccount(accountName)
	if err != nil {
		existing, lookupErr := h.server.LookupAccount(accountName)
		if lookupErr != nil {
			return nil, err
		}
		account = existing
	}
	if h.cfg.JetStream.Enabled && accountName != SystemAccountName && !account.JetStreamEnabled() {
		if err := account.EnableJetStream(defaultJetStreamAccountLimits(), nil); err != nil {
			return nil, fmt.Errorf("enable jetstream for account %q: %w", accountName, err)
		}
	}
	if h.accounts == nil {
		h.accounts = make(map[string]*natsserver.Account)
	}
	h.accounts[accountName] = account
	return account, nil
}

func (h *Hub) registerCredentialLocked(credential Credential) error {
	if !h.started {
		h.cfg.Accounts = appendUserToAccount(h.cfg.Accounts, credential.Account, userConfigFromCredential(credential))
		return nil
	}
	account, err := h.accountLocked(credential.Account)
	if err != nil {
		return err
	}
	if h.credentials == nil {
		h.credentials = newCredentialRegistry()
	}
	if err := h.credentials.add(account, credential); err != nil {
		return err
	}
	h.cfg.Accounts = appendUserToAccount(h.cfg.Accounts, credential.Account, userConfigFromCredential(credential))
	return nil
}

// registerTransientCredentialLocked installs a supervisor-internal credential
// for one running broker instance without turning it into public space state.
func (h *Hub) registerTransientCredentialLocked(credential Credential) error {
	if !h.started {
		return errors.New("nats hub is not started")
	}
	account, err := h.accountLocked(credential.Account)
	if err != nil {
		return err
	}
	if h.credentials == nil {
		h.credentials = newCredentialRegistry()
	}
	return h.credentials.add(account, credential)
}
