package app

import (
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/services/runstate/internal/runstatesvc"
)

const leaseBucket = "runstate_leases"

func openLeaseStore(cfg servicebridge.NATSConfig) (runstatesvc.LeaseStore, func(), error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, nil, fmt.Errorf("nats url is required for runstate leases")
	}
	nc, err := nats.Connect(cfg.URL, nats.UserInfo(cfg.Username, cfg.Password), nats.Name("quark-runstate-leases"))
	if err != nil {
		return nil, nil, fmt.Errorf("connect runstate lease store: %w", err)
	}
	closeConn := func() { nc.Close() }
	js, err := nc.JetStream()
	if err != nil {
		closeConn()
		return nil, nil, fmt.Errorf("open runstate jetstream context: %w", err)
	}
	kv, err := js.KeyValue(leaseBucket)
	if err != nil {
		closeConn()
		return nil, nil, fmt.Errorf("open supervisor-provisioned %s bucket: %w", leaseBucket, err)
	}
	store, err := runstatesvc.NewNATSLeaseStore(kv)
	if err != nil {
		closeConn()
		return nil, nil, err
	}
	return store, closeConn, nil
}
