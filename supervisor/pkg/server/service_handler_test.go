package server

import (
	"os"
	"path/filepath"
	"testing"

	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/space/fsstore"
)

func TestInspectServicesReportsNATSServiceFromManifest(t *testing.T) {
	srv := serviceTestServer(t)
	writeInstalledServicePlugin(t, srv, "test-space")

	services, err := srv.inspectServices(t.Context(), "test-space")
	if err != nil {
		t.Fatalf("inspect services: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("services = %+v", services)
	}
	if services[0].Status != api.ServiceStatusReady || services[0].Endpoint != "svc.indexer.v1" {
		t.Fatalf("service info = %+v", services[0])
	}
}

func serviceTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := fsstore.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("test-space", spacemodel.DefaultQuarkfile("test-space"), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return &Server{store: store}
}

func writeInstalledServicePlugin(t *testing.T, srv *Server, space string) {
	t.Helper()
	mgr, err := srv.store.Plugins(space)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(mgr.PluginsDir(), "services", "indexer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`name: indexer
version: "1.0.0"
type: service
mode: api
description: Indexer service
service:
  address_env: QUARK_INDEXER_ADDR
  health:
    protocol: nats_service
    service: quark.indexer.v1.IndexerService
    timeout: 2s
  readiness:
    required: true
    min_version: "1.0.0"
  skill: SKILL.md
  readme: README.md
  proto_services:
    - quark.indexer.v1.IndexerService
  functions:
    - name: indexer_GetContext
      service: quark.indexer.v1.IndexerService
      method: GetContext
      request: quark.indexer.v1.QueryRequest
      response: quark.indexer.v1.ContextResponse
      description: Retrieve context.
      risk_level: read
      idempotent: true
`)
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# service-indexer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
