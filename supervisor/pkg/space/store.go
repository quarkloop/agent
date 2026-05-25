package space

import (
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/sessions"
)

// Store is the supervisor's semantic space boundary. Authoritative config
// persistence is implemented by calls to the Space service.
type Store interface {
	Create(config []byte) (*Space, error)

	UpdateConfig(config []byte) (*Space, error)

	// Get returns the metadata for the named space.
	Get(name string) (*Space, error)

	// List returns every registered space.
	List() ([]*Space, error)

	// Delete permanently removes the named space and all of its data.
	Delete(name string) error

	// Config returns authoritative `space.json` contents.
	Config(name string) (contents []byte, err error)

	// AgentEnvironment returns concrete environment entries derived from the
	// authoritative space configuration model declaration.
	AgentEnvironment(name string) ([]string, error)

	// Plugins returns the plugin manager scoped to the named space.
	Plugins(name string) (*pluginmanager.Installer, error)

	// Sessions returns the session store scoped to the named space.
	Sessions(name string) (*sessions.Store, error)

	// ServiceStateDir returns a supervisor-owned state directory for a service
	// process scoped to the named space.
	ServiceStateDir(name, service string) (string, error)

	// Doctor runs storage/configuration health checks for a named space.
	Doctor(name string) (api.DoctorResponse, error)
}
