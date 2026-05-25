package natshub

import (
	"context"
)

func (h *Hub) ProvisionSpace(spaceID string) (SpaceCredentials, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.provisionSpaceLocked(spaceID)
}

func (h *Hub) IssueSessionCredential(spaceID, sessionID string) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return Credential{}, err
	}
	key := issuedCredentialKey(RoleSession, spaceID, sessionID)
	if existing, ok := h.issued[key]; ok {
		return cloneCredential(existing), nil
	}
	credential, err := sessionCredential(spaceID, space.Account, sessionID)
	if err != nil {
		return Credential{}, err
	}
	if err := h.registerCredentialLocked(credential); err != nil {
		return Credential{}, err
	}
	h.issued[key] = cloneCredential(credential)
	return cloneCredential(credential), nil
}

func (h *Hub) IssueUserCredential(spaceID string) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return Credential{}, err
	}
	key := issuedCredentialKey(RoleUser, spaceID, "user")
	if existing, ok := h.issued[key]; ok {
		return cloneCredential(existing), nil
	}
	credential, err := userCredential(spaceID, space.Account)
	if err != nil {
		return Credential{}, err
	}
	if err := h.registerCredentialLocked(credential); err != nil {
		return Credential{}, err
	}
	h.issued[key] = cloneCredential(credential)
	return cloneCredential(credential), nil
}

func (h *Hub) IssueRuntimeCredential(spaceID string) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return Credential{}, err
	}
	return cloneCredential(space.Runtime), nil
}

func (h *Hub) IssueAgentCredential(spaceID, agentID string) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return Credential{}, err
	}
	key := issuedCredentialKey(RoleAgent, spaceID, agentID)
	if existing, ok := h.issued[key]; ok {
		return cloneCredential(existing), nil
	}
	credential, err := agentCredential(spaceID, space.Account, agentID)
	if err != nil {
		return Credential{}, err
	}
	if err := h.registerCredentialLocked(credential); err != nil {
		return Credential{}, err
	}
	h.issued[key] = cloneCredential(credential)
	return cloneCredential(credential), nil
}

func (h *Hub) IssueServiceCredential(service string, routes []ServiceFunctionRoute) (Credential, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	normalized, err := NormalizeServiceFunctionRoutes(routes)
	if err != nil {
		return Credential{}, err
	}
	key := issuedCredentialKey(RoleService, ControlAccountName, service)
	if existing, ok := h.issued[key]; ok {
		return cloneCredential(existing), nil
	}
	credential, err := serviceCredential(service, ControlAccountName, normalized)
	if err != nil {
		return Credential{}, err
	}
	if err := h.registerCredentialLocked(credential); err != nil {
		return Credential{}, err
	}
	h.issued[key] = cloneCredential(credential)
	return cloneCredential(credential), nil
}

func (h *Hub) provisionSpaceLocked(spaceID string) (SpaceCredentials, error) {
	if h.cfg.Mode == ModeExternal {
		return SpaceCredentials{}, ErrProvisioningUnavailable
	}
	if existing, ok := h.spaces[spaceID]; ok {
		return cloneSpaceCredentials(existing), nil
	}
	accountName, err := SpaceAccountName(spaceID)
	if err != nil {
		return SpaceCredentials{}, err
	}
	runtimeCred, err := spaceCredential(spaceID, accountName, RoleRuntime, RuntimePermissions())
	if err != nil {
		return SpaceCredentials{}, err
	}
	observabilityCred, err := spaceCredential(spaceID, ObservabilityAccountName, RoleObservability, ObservabilityPermissions(spaceID))
	if err != nil {
		return SpaceCredentials{}, err
	}
	space := SpaceCredentials{
		SpaceID:       spaceID,
		Account:       accountName,
		Runtime:       runtimeCred,
		Observability: observabilityCred,
	}
	if h.started {
		if _, err := h.accountLocked(accountName); err != nil {
			return SpaceCredentials{}, err
		}
		if err := h.registerCredentialLocked(runtimeCred); err != nil {
			return SpaceCredentials{}, err
		}
		if err := h.registerCredentialLocked(observabilityCred); err != nil {
			return SpaceCredentials{}, err
		}
		if err := h.applyCatalogImportsLocked(accountName); err != nil {
			return SpaceCredentials{}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), h.cfg.ReadyTimeout)
		err := h.provisionSpaceRuntimeStorageLocked(ctx, space)
		cancel()
		if err != nil {
			return SpaceCredentials{}, err
		}
	} else {
		h.cfg.Accounts = upsertAccountUsers(h.cfg.Accounts, AccountConfig{
			Name: accountName,
			Users: []UserConfig{
				userConfigFromCredential(runtimeCred),
			},
		})
		h.cfg.Accounts = appendUserToAccount(h.cfg.Accounts, ObservabilityAccountName, userConfigFromCredential(observabilityCred))
	}
	h.spaces[spaceID] = cloneSpaceCredentials(space)
	return cloneSpaceCredentials(space), nil
}
