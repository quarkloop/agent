//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	buildreleasev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/buildrelease/v1"
	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func startIndexerService(t *testing.T, binary, dgraphAddr string) string {
	t.Helper()
	port := utils.ReservePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	startIndexerServiceAt(t, binary, dgraphAddr, addr)
	return addr
}

func startIndexerServiceAt(t *testing.T, binary, dgraphAddr, addr string) {
	t.Helper()
	utils.StartProcess(t, "indexer", binary, []string{
		"--addr", addr,
		"--dgraph", dgraphAddr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "indexer"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, indexerv1.IndexerService_ServiceDesc.ServiceName, 60*time.Second, "indexer")
}

func startEmbeddingServiceAt(t *testing.T, binary, addr string, embedding utils.EmbeddingOptions) {
	t.Helper()
	embedding = embedding.WithDefaults()
	utils.StartProcess(t, "embedding", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", embedding.Plugin),
		"--provider", embedding.Provider,
		"--model", embedding.Model,
		"--dimensions", fmt.Sprint(embedding.Dimensions),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, embeddingv1.EmbeddingService_ServiceDesc.ServiceName, 30*time.Second, "embedding")
}

func startBuildReleaseServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	utils.StartProcess(t, "build-release", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "build-release"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, buildreleasev1.BuildReleaseService_ServiceDesc.ServiceName, 10*time.Second, "build-release")
}

func waitForGRPCHealth(t *testing.T, addr, service string, timeout time.Duration, label string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, err := servicekit.Dial(ctx, addr)
		if err == nil {
			resp, checkErr := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{Service: service})
			conn.Close()
			err = checkErr
			if err == nil && resp.GetStatus() == healthpb.HealthCheckResponse_SERVING {
				cancel()
				return
			}
		}
		cancel()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s service did not become healthy at %s", label, addr)
}
