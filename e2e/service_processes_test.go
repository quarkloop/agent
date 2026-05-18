//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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

const (
	defaultServiceHealthTimeout   = 10 * time.Second
	embeddingServiceHealthTimeout = 30 * time.Second
	indexerServiceHealthTimeout   = 60 * time.Second
)

var modelProviderEnvKeys = []string{
	"OPENROUTER_API_KEY",
	"OPENROUTER_BASE_URL",
	"OPENAI_API_KEY",
	"OPENAI_BASE_URL",
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_BASE_URL",
	"ZHIPU_API_KEY",
	"ZHIPU_BASE_URL",
}

type grpcServiceProcessSpec struct {
	Label         string
	Binary        string
	Address       string
	Plugin        string
	ServiceName   string
	Args          []string
	Env           []string
	HealthTimeout time.Duration
}

func startIndexerService(t *testing.T, binary, dgraphAddr string) string {
	t.Helper()
	addr := reserveLoopbackAddress(t)
	startIndexerServiceAt(t, binary, dgraphAddr, addr)
	return addr
}

func startIndexerServiceAt(t *testing.T, binary, dgraphAddr, addr string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:         "indexer",
		Binary:        binary,
		Address:       addr,
		Plugin:        "indexer",
		ServiceName:   indexerv1.IndexerService_ServiceDesc.ServiceName,
		Args:          []string{"--dgraph", dgraphAddr},
		HealthTimeout: indexerServiceHealthTimeout,
	})
}

func startEmbeddingServiceAt(t *testing.T, binary, addr string, embedding utils.EmbeddingOptions) {
	t.Helper()
	embedding = embedding.WithDefaults()
	env := utils.ServiceProcessEnv(nil)
	if embedding.Provider == "openrouter" {
		env = utils.ServiceProcessEnv(nil, "OPENROUTER_API_KEY", "OPENROUTER_BASE_URL")
	}
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:         "embedding",
		Binary:        binary,
		Address:       addr,
		Plugin:        embedding.Plugin,
		ServiceName:   embeddingv1.EmbeddingService_ServiceDesc.ServiceName,
		Args:          []string{"--provider", embedding.Provider, "--model", embedding.Model, "--dimensions", fmt.Sprint(embedding.Dimensions)},
		Env:           env,
		HealthTimeout: embeddingServiceHealthTimeout,
	})
}

func startDocumentServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:       "document",
		Binary:      binary,
		Address:     addr,
		Plugin:      "document",
		ServiceName: documentv1.DocumentService_ServiceDesc.ServiceName,
	})
}

func startIngestionServiceAt(t *testing.T, binary, addr, root string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:       "ingestion",
		Binary:      binary,
		Address:     addr,
		Plugin:      "ingestion",
		ServiceName: ingestionv1.IngestionService_ServiceDesc.ServiceName,
		Args:        []string{"--root", root},
	})
}

func startCitationServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:       "citation",
		Binary:      binary,
		Address:     addr,
		Plugin:      "citation",
		ServiceName: citationv1.CitationService_ServiceDesc.ServiceName,
	})
}

func startCoreServiceAt(t *testing.T, binary, addr, root string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:       "core",
		Binary:      binary,
		Address:     addr,
		Plugin:      "core",
		ServiceName: corev1.CoreService_ServiceDesc.ServiceName,
		Args:        []string{"--root", root},
	})
}

func startModelServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:       "model",
		Binary:      binary,
		Address:     addr,
		Plugin:      "model",
		ServiceName: modelv1.ModelService_ServiceDesc.ServiceName,
		Env:         utils.ServiceProcessEnv(nil, modelProviderEnvKeys...),
	})
}

func startBuildReleaseServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:       "build-release",
		Binary:      binary,
		Address:     addr,
		Plugin:      "build-release",
		ServiceName: buildreleasev1.BuildReleaseService_ServiceDesc.ServiceName,
	})
}

func startDevOpsServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:       "devops",
		Binary:      binary,
		Address:     addr,
		Plugin:      "devops",
		ServiceName: devopsv1.RepoService_ServiceDesc.ServiceName,
	})
}

func startSystemServiceAt(t *testing.T, binary, addr string) {
	t.Helper()
	startGRPCServiceProcess(t, grpcServiceProcessSpec{
		Label:       "system",
		Binary:      binary,
		Address:     addr,
		Plugin:      "system",
		ServiceName: systemv1.SystemService_ServiceDesc.ServiceName,
	})
}

func standardKnowledgeServicesStartOptions(t *testing.T, embedding utils.EmbeddingOptions, workingDir string) utils.StartOptions {
	t.Helper()
	addresses := reserveKnowledgeServiceAddresses(t)
	return utils.StartOptions{
		WorkingDir:    workingDir,
		Embedding:     embedding,
		Agents:        []string{"quark-knowledge"},
		SupervisorEnv: addresses.supervisorEnv(),
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			dgraphAddr := utils.StartDgraph(t)
			startCoreServiceAt(t, bins.Core, addresses.Core, filepath.Join(setup.SpacesDir, setup.Space, "services", "core"))
			startModelServiceAt(t, bins.Model, addresses.Model)
			startIndexerServiceAt(t, bins.Indexer, dgraphAddr, addresses.Indexer)
			startDocumentServiceAt(t, bins.Document, addresses.Document)
			startIngestionServiceAt(t, bins.Ingestion, addresses.Ingestion, filepath.Join(setup.SpacesDir, setup.Space, "services", "ingestion"))
			startCitationServiceAt(t, bins.Citation, addresses.Citation)
			startEmbeddingServiceAt(t, bins.Embedding, addresses.Embedding, embedding)
		},
	}
}

func standardDevOpsServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	devopsAddr := reserveLoopbackAddress(t)
	buildReleaseAddr := reserveLoopbackAddress(t)
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-devops"},
		ExtraServicePlugins:      []string{"build-release"},
		AgentServicePermissions:  devOpsAgentServicePermissions(),
		Services:                 localServicePlugins("devops", "build-release"),
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

func standardDevOpsOnlyServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	devopsAddr := reserveLoopbackAddress(t)
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-devops"},
		ExtraServicePlugins:      []string{"build-release"},
		AgentServicePermissions:  devOpsAgentServicePermissions(),
		Services:                 localServicePlugins("devops"),
		SupervisorEnv: map[string]string{
			"QUARK_DEVOPS_ADDR": devopsAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startDevOpsServiceAt(t, bins.DevOps, devopsAddr)
		},
	}
}

func standardSystemServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	systemAddr := reserveLoopbackAddress(t)
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-system"},
		Services:                 localServicePlugins("system"),
		SupervisorEnv: map[string]string{
			"QUARK_SYSTEM_ADDR": systemAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startSystemServiceAt(t, bins.System, systemAddr)
		},
	}
}

func startGRPCServiceProcess(t *testing.T, spec grpcServiceProcessSpec) {
	t.Helper()
	if spec.HealthTimeout == 0 {
		spec.HealthTimeout = defaultServiceHealthTimeout
	}
	env := spec.Env
	if env == nil {
		env = utils.ServiceProcessEnv(nil)
	}
	args := []string{
		"--addr", spec.Address,
		"--skill-dir", servicePluginDir(t, spec.Plugin),
	}
	args = append(args, spec.Args...)
	utils.StartProcess(t, spec.Label, spec.Binary, args, env)
	waitForGRPCHealth(t, spec.Address, spec.ServiceName, spec.HealthTimeout, spec.Label)
}

func servicePluginDir(t *testing.T, plugin string) string {
	t.Helper()
	return filepath.Join(utils.QuarkRoot(t), "plugins", "services", plugin)
}

func reserveLoopbackAddress(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
}

type knowledgeServiceAddresses struct {
	Indexer   string
	Embedding string
	Document  string
	Ingestion string
	Citation  string
	Core      string
	Model     string
}

func reserveKnowledgeServiceAddresses(t *testing.T) knowledgeServiceAddresses {
	t.Helper()
	return knowledgeServiceAddresses{
		Indexer:   reserveLoopbackAddress(t),
		Embedding: reserveLoopbackAddress(t),
		Document:  reserveLoopbackAddress(t),
		Ingestion: reserveLoopbackAddress(t),
		Citation:  reserveLoopbackAddress(t),
		Core:      reserveLoopbackAddress(t),
		Model:     reserveLoopbackAddress(t),
	}
}

func (a knowledgeServiceAddresses) supervisorEnv() map[string]string {
	return map[string]string{
		"QUARK_INDEXER_ADDR":       a.Indexer,
		"QUARK_EMBEDDING_ADDR":     a.Embedding,
		"QUARK_DOCUMENT_ADDR":      a.Document,
		"QUARK_INGESTION_ADDR":     a.Ingestion,
		"QUARK_CITATION_ADDR":      a.Citation,
		"QUARK_CORE_ADDR":          a.Core,
		"QUARK_MODEL_SERVICE_ADDR": a.Model,
	}
}

func localServicePlugins(names ...string) []utils.ServicePlugin {
	plugins := make([]utils.ServicePlugin, 0, len(names))
	for _, name := range names {
		plugins = append(plugins, utils.ServicePlugin{
			Name:       name,
			Plugin:     name,
			Mode:       "local",
			AddressEnv: "QUARK_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_ADDR",
		})
	}
	return plugins
}

func devOpsAgentServicePermissions() map[string][]string {
	return map[string][]string{
		"quark-devops": {
			"repo_Status",
			"repo_Diff",
			"repo_GetBranch",
			"repo_ListChangedFiles",
			"build_DetectProject",
			"test_DiscoverTests",
			"test_RunTests",
			"test_ExplainFailure",
			"policy_EvaluateChange",
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
