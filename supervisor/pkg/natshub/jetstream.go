package natshub

import (
	"context"
	"fmt"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

const (
	StreamAudit           = "QUARK_AUDIT"
	StreamTelemetry       = "QUARK_TELEMETRY"
	StreamSessionEvents   = "QUARK_SESSION_EVENTS"
	StreamRuntimeActivity = "QUARK_RUNTIME_ACTIVITY"
	StreamWorkflowEvents  = "QUARK_WORKFLOW_EVENTS"
	StreamCatalog         = "QUARK_CATALOG"
	StreamJobs            = "QUARK_JOBS"

	KVRuntimeSpaceLeases = "runtime_space_leases"
	KVServiceLeases      = "service_leases"
	KVRunStateLeases     = "runstate_leases"
	KVAccountMetadata    = "account_metadata"
	KVCatalogCursors     = "catalog_cursors"

	ObjectArtifactHandoff = "artifact_handoff"
)

type streamSpec struct {
	Name        string
	Description string
	Subjects    []string
	Retention   nats.RetentionPolicy
	MaxAge      time.Duration
	MaxMsgs     int64
}

type kvSpec struct {
	Bucket      string
	Description string
	TTL         time.Duration
}

type objectStoreSpec struct {
	Bucket      string
	Description string
	TTL         time.Duration
	MaxBytes    int64
}

func controlStreams(cfg JetStreamConfig) []streamSpec {
	return []streamSpec{
		{
			Name:        StreamAudit,
			Description: "Redacted append-only audit events.",
			Subjects:    []string{"audit.>"},
			Retention:   nats.LimitsPolicy,
			MaxAge:      cfg.AuditRetention,
			MaxMsgs:     cfg.AuditMaxMessages,
		},
		{
			Name:        StreamTelemetry,
			Description: "Application telemetry events that are also consumed by Vector.",
			Subjects:    []string{"telemetry.>"},
			Retention:   nats.LimitsPolicy,
			MaxAge:      14 * 24 * time.Hour,
			MaxMsgs:     10_000_000,
		},
		{
			Name:        StreamSessionEvents,
			Description: "Session output, status, and client-visible event history.",
			Subjects:    []string{"session.*.events", "session.*.status"},
			Retention:   nats.LimitsPolicy,
			MaxAge:      30 * 24 * time.Hour,
			MaxMsgs:     5_000_000,
		},
		{
			Name:        StreamRuntimeActivity,
			Description: "Runtime and agent activity stream.",
			Subjects:    []string{"runtime.activity.v1.>", "agent.*.events"},
			Retention:   nats.LimitsPolicy,
			MaxAge:      30 * 24 * time.Hour,
			MaxMsgs:     5_000_000,
		},
		{
			Name:        StreamWorkflowEvents,
			Description: "Workflow progress and lifecycle events.",
			Subjects:    []string{"workflow.*.events"},
			Retention:   nats.LimitsPolicy,
			MaxAge:      30 * 24 * time.Hour,
			MaxMsgs:     5_000_000,
		},
		{
			Name:        StreamCatalog,
			Description: "Supervisor-published catalog snapshots and update events.",
			Subjects:    []string{"catalog.runtime.v1.events", "catalog.snapshots.>"},
			Retention:   nats.LimitsPolicy,
			MaxAge:      30 * 24 * time.Hour,
			MaxMsgs:     1_000_000,
		},
		{
			Name:        StreamJobs,
			Description: "JetStream-backed async jobs and work queues.",
			Subjects:    []string{"jobs.>"},
			Retention:   nats.WorkQueuePolicy,
			MaxAge:      7 * 24 * time.Hour,
			MaxMsgs:     1_000_000,
		},
	}
}

func enableJetStreamAccounts(accounts map[string]*natsserver.Account) error {
	for name, account := range accounts {
		if name == SystemAccountName {
			continue
		}
		if err := account.EnableJetStream(defaultJetStreamAccountLimits(), nil); err != nil {
			return fmt.Errorf("enable jetstream for account %q: %w", name, err)
		}
	}
	return nil
}

func defaultJetStreamAccountLimits() map[string]natsserver.JetStreamAccountLimits {
	return map[string]natsserver.JetStreamAccountLimits{
		"": {
			MaxMemory:            -1,
			MaxStore:             -1,
			MaxStreams:           -1,
			MaxConsumers:         -1,
			MaxAckPending:        -1,
			MemoryMaxStreamBytes: -1,
			StoreMaxStreamBytes:  -1,
		},
	}
}

func coordinationBuckets() []kvSpec {
	return []kvSpec{
		{
			Bucket:      KVRuntimeSpaceLeases,
			Description: "Runtime-to-space assignment leases.",
			TTL:         10 * time.Minute,
		},
		{
			Bucket:      KVServiceLeases,
			Description: "Ephemeral service readiness and ownership leases.",
			TTL:         10 * time.Minute,
		},
		{
			Bucket:      KVRunStateLeases,
			Description: "Active run and work-item ownership leases.",
			TTL:         10 * time.Minute,
		},
		{
			Bucket:      KVAccountMetadata,
			Description: "Supervisor-owned account metadata pointers.",
		},
		{
			Bucket:      KVCatalogCursors,
			Description: "Catalog update cursors and compact coordination pointers.",
		},
	}
}

func objectStores(artifactHandoffMaxBytes int64) []objectStoreSpec {
	return []objectStoreSpec{{
		Bucket:      ObjectArtifactHandoff,
		Description: "Temporary large payload handoff for artifacts that should not fit in request/reply envelopes.",
		TTL:         24 * time.Hour,
		MaxBytes:    artifactHandoffMaxBytes,
	}}
}

func (h *Hub) provisionJetStreamLocked(ctx context.Context) error {
	if h == nil || h.server == nil || !h.cfg.JetStream.Enabled {
		return nil
	}
	control, err := h.controlCredentialLocked()
	if err != nil {
		return err
	}
	nc, err := nats.Connect(
		h.server.ClientURL(),
		nats.UserInfo(control.Username, control.Password),
		nats.Name("quark-supervisor-jetstream-provisioner"),
		nats.Timeout(h.cfg.ReadyTimeout),
	)
	if err != nil {
		return fmt.Errorf("connect jetstream provisioner: %w", err)
	}
	defer nc.Close()
	if err := nc.FlushTimeout(h.cfg.ReadyTimeout); err != nil {
		return fmt.Errorf("verify jetstream provisioner: %w", err)
	}
	js, err := nc.JetStream(nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("open jetstream context: %w", err)
	}
	for _, spec := range controlStreams(h.cfg.JetStream) {
		if err := ensureStream(js, spec); err != nil {
			return err
		}
	}
	for _, spec := range coordinationBuckets() {
		if err := ensureBucket(js, spec); err != nil {
			return err
		}
	}
	for _, spec := range objectStores(h.cfg.JetStream.ArtifactHandoffMaxBytes) {
		if err := ensureObjectStore(js, spec); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hub) controlCredentialLocked() (Credential, error) {
	for _, account := range h.cfg.Accounts {
		if account.Name != ControlAccountName {
			continue
		}
		if len(account.Users) == 0 {
			return Credential{}, fmt.Errorf("nats control account has no users")
		}
		user := cloneUserConfig(account.Users[0])
		return Credential{
			Username:    user.Name,
			Password:    user.Password,
			Account:     ControlAccountName,
			Role:        RoleSupervisor,
			Permissions: clonePermissions(user.Permissions),
		}, nil
	}
	return Credential{}, fmt.Errorf("nats control account is not configured")
}

func ensureStream(js nats.JetStreamContext, spec streamSpec) error {
	cfg := &nats.StreamConfig{
		Name:        spec.Name,
		Description: spec.Description,
		Subjects:    append([]string(nil), spec.Subjects...),
		Retention:   spec.Retention,
		Storage:     nats.FileStorage,
		Discard:     nats.DiscardOld,
		MaxAge:      spec.MaxAge,
		MaxMsgs:     spec.MaxMsgs,
		Duplicates:  2 * time.Minute,
	}
	if _, err := js.StreamInfo(spec.Name); err == nil {
		if _, err := js.UpdateStream(cfg); err != nil {
			return fmt.Errorf("update stream %s: %w", spec.Name, err)
		}
		return nil
	}
	if _, err := js.AddStream(cfg); err != nil {
		return fmt.Errorf("create stream %s: %w", spec.Name, err)
	}
	return nil
}

func ensureBucket(js nats.JetStreamContext, spec kvSpec) error {
	if _, err := js.KeyValue(spec.Bucket); err == nil {
		return nil
	}
	_, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      spec.Bucket,
		Description: spec.Description,
		History:     1,
		TTL:         spec.TTL,
		Storage:     nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create kv bucket %s: %w", spec.Bucket, err)
	}
	return nil
}

func ensureObjectStore(js nats.JetStreamContext, spec objectStoreSpec) error {
	if _, err := js.ObjectStore(spec.Bucket); err == nil {
		return nil
	}
	_, err := js.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket:      spec.Bucket,
		Description: spec.Description,
		TTL:         spec.TTL,
		MaxBytes:    spec.MaxBytes,
		Storage:     nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create object store %s: %w", spec.Bucket, err)
	}
	return nil
}
