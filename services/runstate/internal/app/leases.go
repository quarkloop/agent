package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/services/runstate/internal/runstatesvc"
)

const leaseBucket = "runstate_leases"

func openLeaseStore(ctx context.Context, cfg natskit.Config) (runstatesvc.LeaseStore, func(), error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, nil, fmt.Errorf("nats url is required for runstate leases")
	}
	cfg.Name = "quark-runstate-leases"
	client, err := natskit.Connect(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("connect runstate lease store: %w", err)
	}
	closeConn := client.Close
	kv, err := client.OpenKeyValue(leaseBucket)
	if err != nil {
		closeConn()
		return nil, nil, fmt.Errorf("open supervisor-provisioned %s bucket: %w", leaseBucket, err)
	}
	store, err := runstatesvc.NewCASLeaseStore(natsLeaseCASStore{kv: kv})
	if err != nil {
		closeConn()
		return nil, nil, err
	}
	return store, closeConn, nil
}

type natsLeaseCASStore struct {
	kv *natskit.KeyValue
}

func (s natsLeaseCASStore) Create(key string, value []byte) (uint64, error) {
	return s.kv.Create(key, value)
}

func (s natsLeaseCASStore) Get(key string) (runstatesvc.CASEntry, error) {
	entry, err := s.kv.Get(key)
	if errors.Is(err, natskit.ErrKeyNotFound) {
		return runstatesvc.CASEntry{}, runstatesvc.ErrCASKeyNotFound
	}
	if err != nil {
		return runstatesvc.CASEntry{}, err
	}
	return runstatesvc.CASEntry{Value: entry.Value(), Revision: entry.Revision()}, nil
}

func (s natsLeaseCASStore) Update(key string, value []byte, revision uint64) (uint64, error) {
	return s.kv.Update(key, value, revision)
}

func (s natsLeaseCASStore) DeleteRevision(key string, revision uint64) error {
	return s.kv.DeleteRevision(key, revision)
}
