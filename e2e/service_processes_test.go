//go:build e2e

package e2e

import (
	"fmt"
	"os"
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
)

var gatewayProviderEnvKeys = []string{
	"QUARK_GATEWAY_TIMEOUT",
	"QUARK_OPENROUTER_PROVIDER_KIND",
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
	args = append(args, "--nats-timeout", e2eGatewayTimeout())
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

func e2eGatewayTimeout() string {
	if value := strings.TrimSpace(os.Getenv("QUARK_E2E_MODEL_GATEWAY_TIMEOUT")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("QUARK_GATEWAY_TIMEOUT")); value != "" {
		return value
	}
	return "2m"
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

func standardKnowledgeServicesStartOptions(t *testing.T, embedding utils.EmbeddingOptions, workingDir string) utils.StartOptions {
	t.Helper()
	addresses := reserveKnowledgeServiceAddresses(t)
	return utils.StartOptions{
		WorkingDir:              workingDir,
		Embedding:               embedding,
		Agents:                  []string{"quark-main"},
		AgentServicePermissions: knowledgeAgentServicePermissions(),
		SupervisorEnv:           addresses.supervisorEnv(),
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
	ioAddr := reserveLoopbackAddress(t)
	gatewayAddr := reserveLoopbackAddress(t)
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-main"},
		AgentServicePermissions:  devOpsAgentServicePermissions(devOpsReleaseServiceFunctions()...),
		Services:                 append(localServicePlugins("devops", "io"), gatewayServicePlugin()),
		SupervisorEnv: map[string]string{
			"QUARK_DEVOPS_ADDR":          devopsAddr,
			"QUARK_IO_ADDR":              ioAddr,
			"QUARK_GATEWAY_SERVICE_ADDR": gatewayAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startIOServiceAt(t, bins.IO, ioAddr, setup.NATS)
			startGatewayServiceAt(t, bins.Model, gatewayAddr, setup.NATS.ClientURL)
			startDevOpsServiceAt(t, bins.DevOps, devopsAddr, setup.NATS)
		},
	}
}

func standardDevOpsOnlyServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	devopsAddr := reserveLoopbackAddress(t)
	gatewayAddr := reserveLoopbackAddress(t)
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-main"},
		AgentServicePermissions:  devOpsAgentServicePermissions(),
		Services:                 append(localServicePlugins("devops"), gatewayServicePlugin()),
		SupervisorEnv: map[string]string{
			"QUARK_DEVOPS_ADDR":          devopsAddr,
			"QUARK_GATEWAY_SERVICE_ADDR": gatewayAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startGatewayServiceAt(t, bins.Model, gatewayAddr, setup.NATS.ClientURL)
			startDevOpsServiceAt(t, bins.DevOps, devopsAddr, setup.NATS)
		},
	}
}

func standardSystemServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	systemAddr := reserveLoopbackAddress(t)
	gatewayAddr := reserveLoopbackAddress(t)
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-main"},
		AgentServicePermissions:  systemReadOnlyAgentServicePermissions(),
		Services:                 append(localServicePlugins("system"), gatewayServicePlugin()),
		SupervisorEnv: map[string]string{
			"QUARK_SYSTEM_ADDR":          systemAddr,
			"QUARK_GATEWAY_SERVICE_ADDR": gatewayAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startGatewayServiceAt(t, bins.Model, gatewayAddr, setup.NATS.ClientURL)
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

func gatewayServicePlugin() utils.ServicePlugin {
	return utils.ServicePlugin{
		Name:       "gateway",
		Plugin:     "gateway",
		Mode:       "local",
		AddressEnv: "QUARK_GATEWAY_SERVICE_ADDR",
	}
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
		"quark-main": allowed,
	}
}

func knowledgeAgentServicePermissions() map[string][]string {
	return map[string][]string{
		"quark-main": knowledgeServiceFunctions(),
	}
}

func systemReadOnlyAgentServicePermissions() map[string][]string {
	return map[string][]string{
		"quark-main": {
			"system_Snapshot",
			"system_GetOSInfo",
			"system_GetKernelInfo",
			"system_GetUptime",
			"system_ListPackages",
			"system_ListServices",
			"system_ListUsers",
			"system_ListMounts",
			"system_GetDiskUsage",
			"system_ListProcesses",
			"system_ListPorts",
			"system_ListNetworkConnections",
			"system_ReadLogs",
			"system_GetMetrics",
		},
	}
}

func knowledgeServiceFunctions() []string {
	return []string{
		"io_Read",
		"io_List",
		"io_Stat",
		"io_ExtractPdf",
		"io_Write",
		"io_Append",
		"io_Replace",
		"io_Remove",
		"document_DetectType",
		"document_ParseBytes",
		"document_ExtractText",
		"document_ExtractLayout",
		"document_GetPages",
		"document_ExtractTables",
		"document_ExtractImages",
		"document_RunOCR",
		"ingestion_StartRun",
		"ingestion_GetRun",
		"ingestion_ListRuns",
		"ingestion_ResumeRun",
		"ingestion_UpdateSourceState",
		"ingestion_AppendArtifact",
		"ingestion_MarkFailed",
		"ingestion_MarkComplete",
		"ingestion_CancelRun",
		"ingestion_ListIncompleteSources",
		"ingestion_ListArtifacts",
		"embedding_Embed",
		"indexer_UpsertChunk",
		"indexer_UpsertFact",
		"indexer_UpsertEntity",
		"indexer_UpsertRelation",
		"indexer_UpsertCitation",
		"indexer_IndexDocument",
		"indexer_QueryContext",
		"indexer_GetContext",
		"indexer_DeleteDocument",
		"indexer_DeleteChunk",
		"citation_ResolveSpans",
		"citation_CreateCitation",
		"citation_VerifyGrounding",
		"citation_ScoreCoverage",
		"citation_RenderReferences",
		"core_CreateWorkspaceMutationPlan",
		"core_ApproveWorkspaceMutationPlan",
		"core_RequestApproval",
		"core_EvaluatePolicy",
		"core_RecordAuditEvent",
		"core_PutArtifact",
	}
}

func devOpsReleaseServiceFunctions() []string {
	return []string{
		"build_DryRunRelease",
		"build_InitReleaseConfig",
		"build_RunRelease",
	}
}

func TestKnowledgeAgentServicePermissionsMatchStandardE2EStack(t *testing.T) {
	permissions := knowledgeAgentServicePermissions()["quark-main"]
	required := []string{
		"io_Read",
		"document_ExtractText",
		"ingestion_StartRun",
		"embedding_Embed",
		"indexer_IndexDocument",
		"citation_VerifyGrounding",
		"core_RecordAuditEvent",
	}
	seen := make(map[string]struct{}, len(permissions))
	for _, function := range permissions {
		if strings.HasPrefix(function, "workflow_") {
			t.Fatalf("standard knowledge e2e stack must not require workflow deployment: %s", function)
		}
		seen[function] = struct{}{}
	}
	for _, function := range required {
		if _, ok := seen[function]; !ok {
			t.Fatalf("standard knowledge e2e stack missing required service function %s", function)
		}
	}
}
