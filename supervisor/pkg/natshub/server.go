package natshub

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func (h *Hub) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return errors.New("nats hub is already started")
	}
	if h.cfg.Mode == ModeExternal {
		h.started = true
		return nil
	}
	if err := os.MkdirAll(h.cfg.StateDir, 0o755); err != nil {
		return fmt.Errorf("create nats state dir: %w", err)
	}
	if h.cfg.JetStream.Enabled {
		if err := os.MkdirAll(h.cfg.JetStream.StoreDir, 0o755); err != nil {
			return fmt.Errorf("create nats jetstream dir: %w", err)
		}
	}
	opts, configuredAccounts, credentials, err := buildOptionsAndRegistry(h.cfg)
	if err != nil {
		return err
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		return fmt.Errorf("create embedded nats server: %w", err)
	}
	srv.Start()
	if !readyForConnections(ctx, srv, h.cfg.ReadyTimeout) {
		srv.Shutdown()
		srv.WaitForShutdown()
		return errors.New("embedded nats server did not become ready")
	}
	accounts, err := registeredAccounts(srv, configuredAccounts)
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		return err
	}
	if h.cfg.JetStream.Enabled {
		if err := enableJetStreamAccounts(accounts); err != nil {
			srv.Shutdown()
			srv.WaitForShutdown()
			return err
		}
	}
	if err := credentials.rebindAccounts(accounts); err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		return err
	}
	h.server = srv
	h.accounts = accounts
	h.credentials = credentials
	h.started = true
	h.applied = make(map[string]map[string]struct{})
	if err := h.provisionJetStreamLocked(ctx); err != nil {
		h.shutdownStartedServerLocked()
		return err
	}
	for _, space := range h.spaces {
		if err := h.applyCatalogImportsLocked(space.Account); err != nil {
			h.shutdownStartedServerLocked()
			return err
		}
		if err := h.provisionSpaceRuntimeStorageLocked(ctx, space); err != nil {
			h.shutdownStartedServerLocked()
			return err
		}
	}
	for spaceID, routes := range h.imports {
		space, ok := h.spaces[spaceID]
		if !ok {
			h.shutdownStartedServerLocked()
			return fmt.Errorf("space %q has service imports but no provisioned account", spaceID)
		}
		if err := h.applyServiceFunctionImportsLocked(space.Account, routes); err != nil {
			h.shutdownStartedServerLocked()
			return err
		}
	}
	return nil
}

func registeredAccounts(srv *natsserver.Server, configured map[string]*natsserver.Account) (map[string]*natsserver.Account, error) {
	out := make(map[string]*natsserver.Account, len(configured))
	for name := range configured {
		account, err := srv.LookupAccount(name)
		if err != nil {
			return nil, fmt.Errorf("lookup registered nats account %q: %w", name, err)
		}
		out[name] = account
	}
	return out, nil
}

func (h *Hub) shutdownStartedServerLocked() {
	srv := h.server
	h.server = nil
	h.started = false
	h.accounts = nil
	h.credentials = nil
	h.applied = make(map[string]map[string]struct{})
	if srv != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
	}
}

func (h *Hub) Stop(ctx context.Context) error {
	h.mu.Lock()
	srv := h.server
	h.server = nil
	h.started = false
	h.accounts = nil
	h.credentials = nil
	h.applied = make(map[string]map[string]struct{})
	h.mu.Unlock()

	if srv == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		srv.Shutdown()
		srv.WaitForShutdown()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (h *Hub) ServerForTest() *natsserver.Server {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.server
}

func readyForConnections(ctx context.Context, srv *natsserver.Server, timeoutDuration time.Duration) bool {
	done := make(chan bool, 1)
	go func() {
		done <- srv.ReadyForConnections(timeoutDuration)
	}()
	select {
	case <-ctx.Done():
		return false
	case ok := <-done:
		return ok
	}
}
