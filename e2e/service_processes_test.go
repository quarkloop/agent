//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

const (
	defaultServiceReadyTimeout   = 10 * time.Second
	embeddingServiceReadyTimeout = 30 * time.Second
	indexerServiceReadyTimeout   = 60 * time.Second
	natsServiceBridgeReadyLog    = "nats service bridge ready"
	gatewayServiceReadyLog       = "gateway service listening"
	workflowServiceReadyLog      = "workflow service listening"
	secretsServiceReadyLog       = "secrets service listening"
)

var gatewayProviderEnvKeys = []string{
	"OPENROUTER_API_KEY",
	"OPENROUTER_BASE_URL",
	"OPENAI_API_KEY",
	"OPENAI_BASE_URL",
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_BASE_URL",
	"ZHIPU_API_KEY",
	"ZHIPU_BASE_URL",
}

type natsServiceProcessSpec struct {
	Label        string
	Binary       string
	Address      string
	Plugin       string
	NATS         utils.NATSEndpoints
	Args         []string
	Env          []string
	ReadyLog     string
	ReadyTimeout time.Duration
}

func startIndexerServiceAt(t *testing.T, binary, dgraphAddr, addr string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:        "indexer",
		Binary:       binary,
		Address:      addr,
		Plugin:       "indexer",
		NATS:         nats,
		Args:         []string{"--dgraph", dgraphAddr},
		ReadyTimeout: indexerServiceReadyTimeout,
	})
}

func startEmbeddingServiceAt(t *testing.T, binary, addr string, embedding utils.EmbeddingOptions, nats utils.NATSEndpoints) {
	t.Helper()
	embedding = embedding.WithDefaults()
	env := utils.ServiceProcessEnv(nil)
	if embedding.Provider == "openrouter" {
		env = utils.ServiceProcessEnv(nil, "OPENROUTER_API_KEY", "OPENROUTER_BASE_URL")
	}
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:        "embedding",
		Binary:       binary,
		Address:      addr,
		Plugin:       embedding.Plugin,
		NATS:         nats,
		Args:         []string{"--provider", embedding.Provider, "--model", embedding.Model, "--dimensions", fmt.Sprint(embedding.Dimensions)},
		Env:          env,
		ReadyTimeout: embeddingServiceReadyTimeout,
	})
}

func startIOServiceAt(t *testing.T, binary, addr string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:   "io",
		Binary:  binary,
		Address: addr,
		Plugin:  "io",
		NATS:    nats,
	})
}

func startDocumentServiceAt(t *testing.T, binary, addr string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:   "document",
		Binary:  binary,
		Address: addr,
		Plugin:  "document",
		NATS:    nats,
	})
}

func startIngestionServiceAt(t *testing.T, binary, addr, root string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:   "ingestion",
		Binary:  binary,
		Address: addr,
		Plugin:  "ingestion",
		NATS:    nats,
		Args:    []string{"--root", root},
	})
}

func startCitationServiceAt(t *testing.T, binary, addr string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:   "citation",
		Binary:  binary,
		Address: addr,
		Plugin:  "citation",
		NATS:    nats,
	})
}

func startCoreServiceAt(t *testing.T, binary, addr, root string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:   "core",
		Binary:  binary,
		Address: addr,
		Plugin:  "core",
		NATS:    nats,
		Args:    []string{"--root", root},
	})
}

func startGatewayServiceAt(t *testing.T, binary, addr, natsURL string) {
	t.Helper()
	args := []string{}
	if natsURL != "" {
		args = append(args, "--nats-url", natsURL, "--nats-user", natshub.DefaultControlUser, "--nats-password", natshub.DefaultControlPassword)
	}
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:    "gateway",
		Binary:   binary,
		Address:  addr,
		Plugin:   "gateway",
		Args:     args,
		Env:      utils.ServiceProcessEnv(nil, gatewayProviderEnvKeys...),
		ReadyLog: gatewayServiceReadyLog,
	})
}

func startBuildReleaseServiceAt(t *testing.T, binary, addr string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:   "build-release",
		Binary:  binary,
		Address: addr,
		Plugin:  "build-release",
		NATS:    nats,
	})
}

func startDevOpsServiceAt(t *testing.T, binary, addr string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:   "devops",
		Binary:  binary,
		Address: addr,
		Plugin:  "devops",
		NATS:    nats,
	})
}

func startSystemServiceAt(t *testing.T, binary, addr string, nats utils.NATSEndpoints) {
	t.Helper()
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:   "system",
		Binary:  binary,
		Address: addr,
		Plugin:  "system",
		NATS:    nats,
	})
}

func startWorkflowServiceAt(t *testing.T, binary, addr, temporalAddr, natsURL string) {
	t.Helper()
	args := []string{"--temporal-addr", temporalAddr}
	if natsURL != "" {
		args = append(args, "--nats-url", natsURL, "--nats-user", natshub.DefaultControlUser, "--nats-password", natshub.DefaultControlPassword)
	}
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:    "workflow",
		Binary:   binary,
		Address:  addr,
		Plugin:   "workflow",
		Args:     args,
		ReadyLog: workflowServiceReadyLog,
	})
}

func startSecretsServiceAt(t *testing.T, binary, addr, openBaoAddr, token, natsURL string) {
	t.Helper()
	args := []string{"--openbao-addr", openBaoAddr, "--openbao-token", token}
	if natsURL != "" {
		args = append(args, "--nats-url", natsURL, "--nats-user", natshub.DefaultControlUser, "--nats-password", natshub.DefaultControlPassword)
	}
	startNATSServiceProcess(t, natsServiceProcessSpec{
		Label:    "secrets",
		Binary:   binary,
		Address:  addr,
		Plugin:   "secrets",
		Args:     args,
		ReadyLog: secretsServiceReadyLog,
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
			startIOServiceAt(t, bins.IO, addresses.IO, setup.NATS)
			startCoreServiceAt(t, bins.Core, addresses.Core, filepath.Join(setup.SpacesDir, setup.Space, "services", "core"), setup.NATS)
			startGatewayServiceAt(t, bins.Model, addresses.Gateway, setup.NATS.ClientURL)
			startIndexerServiceAt(t, bins.Indexer, dgraphAddr, addresses.Indexer, setup.NATS)
			startDocumentServiceAt(t, bins.Document, addresses.Document, setup.NATS)
			startIngestionServiceAt(t, bins.Ingestion, addresses.Ingestion, filepath.Join(setup.SpacesDir, setup.Space, "services", "ingestion"), setup.NATS)
			startCitationServiceAt(t, bins.Citation, addresses.Citation, setup.NATS)
			startEmbeddingServiceAt(t, bins.Embedding, addresses.Embedding, embedding, setup.NATS)
		},
	}
}

func standardDevOpsServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	devopsAddr := reserveLoopbackAddress(t)
	buildReleaseAddr := reserveLoopbackAddress(t)
	ioAddr := reserveLoopbackAddress(t)
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-devops"},
		ExtraServicePlugins:      []string{"build-release"},
		AgentServicePermissions:  devOpsAgentServicePermissions(buildReleaseServiceFunctions()...),
		Services:                 localServicePlugins("devops", "build-release", "io"),
		SupervisorEnv: map[string]string{
			"QUARK_DEVOPS_ADDR":        devopsAddr,
			"QUARK_BUILD_RELEASE_ADDR": buildReleaseAddr,
			"QUARK_IO_ADDR":            ioAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startIOServiceAt(t, bins.IO, ioAddr, setup.NATS)
			startDevOpsServiceAt(t, bins.DevOps, devopsAddr, setup.NATS)
			startBuildReleaseServiceAt(t, bins.BuildRelease, buildReleaseAddr, setup.NATS)
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
			startDevOpsServiceAt(t, bins.DevOps, devopsAddr, setup.NATS)
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
			startSystemServiceAt(t, bins.System, systemAddr, setup.NATS)
		},
	}
}

func startNATSServiceProcess(t *testing.T, spec natsServiceProcessSpec) {
	t.Helper()
	if spec.ReadyTimeout == 0 {
		spec.ReadyTimeout = defaultServiceReadyTimeout
	}
	if spec.ReadyLog == "" {
		spec.ReadyLog = natsServiceBridgeReadyLog
	}
	env := spec.Env
	if env == nil {
		env = utils.ServiceProcessEnv(nil)
	}
	args := []string{
		"--addr", spec.Address,
		"--skill-dir", servicePluginDir(t, spec.Plugin),
	}
	if spec.NATS.ClientURL != "" {
		args = append(args,
			"--nats-url", spec.NATS.ClientURL,
			"--nats-user", natshub.DefaultControlUser,
			"--nats-password", natshub.DefaultControlPassword,
		)
	}
	args = append(args, spec.Args...)
	process := utils.StartProcess(t, spec.Label, spec.Binary, args, env)
	process.WaitForLog(t, spec.ReadyLog, spec.ReadyTimeout)
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
	Gateway   string
	IO        string
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
		Gateway:   reserveLoopbackAddress(t),
		IO:        reserveLoopbackAddress(t),
	}
}

func (a knowledgeServiceAddresses) supervisorEnv() map[string]string {
	return map[string]string{
		"QUARK_INDEXER_ADDR":         a.Indexer,
		"QUARK_EMBEDDING_ADDR":       a.Embedding,
		"QUARK_DOCUMENT_ADDR":        a.Document,
		"QUARK_INGESTION_ADDR":       a.Ingestion,
		"QUARK_CITATION_ADDR":        a.Citation,
		"QUARK_CORE_ADDR":            a.Core,
		"QUARK_GATEWAY_SERVICE_ADDR": a.Gateway,
		"QUARK_IO_ADDR":              a.IO,
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

func devOpsAgentServicePermissions(extra ...string) map[string][]string {
	allowed := []string{
		"repo_Status",
		"repo_Diff",
		"repo_GetBranch",
		"repo_ListChangedFiles",
		"build_DetectProject",
		"test_DiscoverTests",
		"test_RunTests",
		"test_ExplainFailure",
		"policy_EvaluateChange",
	}
	allowed = append(allowed, extra...)
	return map[string][]string{
		"quark-devops": allowed,
	}
}

func buildReleaseServiceFunctions() []string {
	return []string{
		"build_release_DryRun",
		"build_release_Init",
		"build_release_Release",
	}
}
