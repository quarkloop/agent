package sessions

import (
	"errors"
	"testing"
	"time"

	spacedomain "github.com/quarkloop/supervisor/pkg/space"
)

func TestRepositoryPersistsSessionRecordsThroughStorageBoundary(t *testing.T) {
	records := &recordFixture{values: make(map[string][]byte)}
	repository, err := NewRepository(records)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return now }
	repository.id = func() string { return "session-one" }

	created, err := repository.Create("docs", TypeChat, "research")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "session-one" || records.lastNamespace != recordNamespace {
		t.Fatalf("created = %+v namespace = %q", created, records.lastNamespace)
	}
	got, err := repository.Get("docs", created.ID)
	if err != nil || got.Title != "research" {
		t.Fatalf("get = %+v err = %v", got, err)
	}
	items, err := repository.List("docs")
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("list = %+v err = %v", items, err)
	}
	if err := repository.Delete("docs", created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get("docs", created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted get error = %v", err)
	}
}

type recordFixture struct {
	values        map[string][]byte
	lastNamespace string
}

func (r *recordFixture) PutRecord(space, namespace, key string, data []byte) error {
	r.lastNamespace = namespace
	r.values[space+"/"+namespace+"/"+key] = append([]byte(nil), data...)
	return nil
}

func (r *recordFixture) GetRecord(space, namespace, key string) ([]byte, error) {
	data, exists := r.values[space+"/"+namespace+"/"+key]
	if !exists {
		return nil, spacedomain.NewNotFoundError(key)
	}
	return append([]byte(nil), data...), nil
}

func (r *recordFixture) ListRecords(space, namespace string) ([][]byte, error) {
	data, exists := r.values[space+"/"+namespace+"/session-one"]
	if !exists {
		return nil, nil
	}
	return [][]byte{append([]byte(nil), data...)}, nil
}

func (r *recordFixture) DeleteRecord(space, namespace, key string) error {
	recordKey := space + "/" + namespace + "/" + key
	if _, exists := r.values[recordKey]; !exists {
		return spacedomain.NewNotFoundError(key)
	}
	delete(r.values, recordKey)
	return nil
}
