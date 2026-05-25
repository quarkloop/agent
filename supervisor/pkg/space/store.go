package space

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

	// PutRecord persists opaque bytes under a caller-owned namespace. The
	// Space service never interprets record content.
	PutRecord(name, namespace, key string, data []byte) error
	GetRecord(name, namespace, key string) ([]byte, error)
	ListRecords(name, namespace string) ([][]byte, error)
	DeleteRecord(name, namespace, key string) error

	// Doctor runs storage/configuration health checks for a named space.
	Doctor(name string) (DoctorResult, error)
}
