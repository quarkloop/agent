package natshub

import (
	"sync"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// Hub owns broker lifecycle and broker-side account/resource provisioning.
// Application protocols are served outside this package through pkg/natskit.
type Hub struct {
	cfg Config

	mu          sync.Mutex
	server      *natsserver.Server
	started     bool
	accounts    map[string]*natsserver.Account
	credentials *credentialRegistry
	spaces      map[string]SpaceCredentials
	issued      map[string]Credential
	imports     map[string][]ServiceFunctionRoute
	applied     map[string]map[string]struct{}
}

const (
	catalogRuntimeGetSubject    = "catalog.runtime.v1.get"
	catalogRuntimeEventsSubject = "catalog.runtime.v1.events"
)

func New(cfg Config) (*Hub, error) {
	normalized, err := Normalize(cfg)
	if err != nil {
		return nil, err
	}
	return &Hub{
		cfg:     normalized,
		spaces:  make(map[string]SpaceCredentials),
		issued:  make(map[string]Credential),
		imports: make(map[string][]ServiceFunctionRoute),
		applied: make(map[string]map[string]struct{}),
	}, nil
}

func (h *Hub) Config() Config {
	h.mu.Lock()
	defer h.mu.Unlock()
	return cloneConfig(h.cfg)
}
