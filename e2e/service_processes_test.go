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
	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	ingestionv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/ingestion/v1"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
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

func startDocumentServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	utils.StartProcess(t, "document", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "document"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, documentv1.DocumentService_ServiceDesc.ServiceName, 10*time.Second, "document")
}

func startIngestionServiceAt(t *testing.T, binary, addr, root string) {
	t.Helper()
	utils.StartProcess(t, "ingestion", binary, []string{
		"--addr", addr,
		"--root", root,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "ingestion"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, ingestionv1.IngestionService_ServiceDesc.ServiceName, 10*time.Second, "ingestion")
}

func startCitationServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	utils.StartProcess(t, "citation", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "citation"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, citationv1.CitationService_ServiceDesc.ServiceName, 10*time.Second, "citation")
}

func startCoreServiceAt(t *testing.T, binary, addr, root string) {
	t.Helper()
	utils.StartProcess(t, "core", binary, []string{
		"--addr", addr,
		"--root", root,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "core"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, corev1.CoreService_ServiceDesc.ServiceName, 10*time.Second, "core")
}

func startModelServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	utils.StartProcess(t, "model", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "model"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, modelv1.ModelService_ServiceDesc.ServiceName, 10*time.Second, "model")
}

func startBuildReleaseServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	utils.StartProcess(t, "build-release", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "build-release"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, buildreleasev1.BuildReleaseService_ServiceDesc.ServiceName, 10*time.Second, "build-release")
}

func startDevOpsServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	utils.StartProcess(t, "devops", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "devops"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, devopsv1.RepoService_ServiceDesc.ServiceName, 10*time.Second, "devops")
}

func startSystemServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	utils.StartProcess(t, "system", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", "system"),
	}, utils.ProcessEnv(nil))
	waitForGRPCHealth(t, addr, systemv1.SystemService_ServiceDesc.ServiceName, 10*time.Second, "system")
}

func standardKnowledgeServicesStartOptions(t *testing.T, embedding utils.EmbeddingOptions, workingDir string) utils.StartOptions {
	t.Helper()
	indexerAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	embeddingAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	documentAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	ingestionAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	citationAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	coreAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	modelAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	return utils.StartOptions{
		WorkingDir: workingDir,
		Embedding:  embedding,
		Agents:     []string{"quark-knowledge"},
		SupervisorEnv: map[string]string{
			"QUARK_INDEXER_ADDR":       indexerAddr,
			"QUARK_EMBEDDING_ADDR":     embeddingAddr,
			"QUARK_DOCUMENT_ADDR":      documentAddr,
			"QUARK_INGESTION_ADDR":     ingestionAddr,
			"QUARK_CITATION_ADDR":      citationAddr,
			"QUARK_CORE_ADDR":          coreAddr,
			"QUARK_MODEL_SERVICE_ADDR": modelAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			dgraphAddr := utils.StartDgraph(t)
			startCoreServiceAt(t, bins.Core, coreAddr, filepath.Join(setup.SpacesDir, setup.Space, "services", "core"))
			startModelServiceAt(t, bins.Model, modelAddr)
			startIndexerServiceAt(t, bins.Indexer, dgraphAddr, indexerAddr)
			startDocumentServiceAt(t, bins.Document, documentAddr)
			startIngestionServiceAt(t, bins.Ingestion, ingestionAddr, filepath.Join(setup.SpacesDir, setup.Space, "services", "ingestion"))
			startCitationServiceAt(t, bins.Citation, citationAddr)
			startEmbeddingServiceAt(t, bins.Embedding, embeddingAddr, embedding)
		},
	}
}

func standardDevOpsServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	devopsAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	buildReleaseAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-devops"},
		Services: []utils.ServicePlugin{
			{
				Name:       "devops",
				Plugin:     "devops",
				Mode:       "local",
				AddressEnv: "QUARK_DEVOPS_ADDR",
			},
			{
				Name:       "build-release",
				Plugin:     "build-release",
				Mode:       "local",
				AddressEnv: "QUARK_BUILD_RELEASE_ADDR",
			},
		},
		SupervisorEnv: map[string]string{
			"QUARK_DEVOPS_ADDR":        devopsAddr,
			"QUARK_BUILD_RELEASE_ADDR": buildReleaseAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startDevOpsServiceAt(t, bins.DevOps, devopsAddr)
			startBuildReleaseServiceAt(t, bins.BuildRelease, buildReleaseAddr)
		},
	}
}

func standardSystemServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	systemAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-system"},
		Services: []utils.ServicePlugin{{
			Name:       "system",
			Plugin:     "system",
			Mode:       "local",
			AddressEnv: "QUARK_SYSTEM_ADDR",
		}},
		SupervisorEnv: map[string]string{
			"QUARK_SYSTEM_ADDR": systemAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startSystemServiceAt(t, bins.System, systemAddr)
		},
	}
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
