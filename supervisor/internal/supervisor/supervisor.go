package supervisor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const supervisorStateFile = "/tmp/supervisor/supervisor.json"

type State struct {
	PID int `json:"pid"`
}

type Supervisor struct {
	path string
}

func New() *Supervisor {
	return &Supervisor{path: supervisorStateFile}
}

func (r *Supervisor) Save(state State) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
		return err
	}

	f, err := os.Create(r.path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(state)
}

func (r *Supervisor) Load() (State, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, errors.New("supervisor is not running")
		}
		return State{}, err
	}
	defer f.Close()

	var s State
	return s, json.NewDecoder(f).Decode(&s)
}

func (r *Supervisor) Clear() error {
	return os.Remove(r.path)
}
