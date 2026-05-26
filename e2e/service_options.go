//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func standardKnowledgeServicesStartOptions(t *testing.T, embedding utils.GatewayEmbeddingOptions, workingDir string) utils.StartOptions {
	t.Helper()
	return utils.StartOptions{
		WorkingDir:              workingDir,
		Embedding:               embedding,
		Agents:                  []string{"quark-main"},
		AgentServicePermissions: knowledgeAgentServicePermissions(),
	}
}

func standardDevOpsServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-main"},
		AgentServicePermissions:  devOpsAgentServicePermissions(devOpsReleaseServiceFunctions()...),
		Services:                 append(localServicePlugins("devops", "io"), gatewayServicePlugin()),
	}
}

func standardDevOpsOnlyServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-main"},
		AgentServicePermissions:  devOpsAgentServicePermissions(),
		Services:                 append(localServicePlugins("devops"), gatewayServicePlugin()),
	}
}

func standardSystemServicesStartOptions(t *testing.T, workingDir string) utils.StartOptions {
	t.Helper()
	return utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-main"},
		AgentServicePermissions:  systemReadOnlyAgentServicePermissions(),
		Services:                 append(localServicePlugins("system"), gatewayServicePlugin()),
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
	return utils.ServicePlugin{Name: "gateway", Plugin: "gateway", Mode: "local", AddressEnv: "QUARK_GATEWAY_SERVICE_ADDR"}
}

func devOpsAgentServicePermissions(extra ...string) map[string][]string {
	allowed := []string{
		"repo_Status", "repo_Diff", "repo_GetBranch", "repo_ListChangedFiles",
		"build_DetectProject", "test_DiscoverTests", "test_RunTests",
		"test_ExplainFailure", "policy_EvaluateChange",
	}
	allowed = append(allowed, extra...)
	return map[string][]string{"quark-main": allowed}
}

func knowledgeAgentServicePermissions() map[string][]string {
	return map[string][]string{"quark-main": knowledgeServiceFunctions()}
}

func systemReadOnlyAgentServicePermissions() map[string][]string {
	return map[string][]string{"quark-main": {
		"system_Snapshot", "system_GetOSInfo", "system_GetKernelInfo",
		"system_GetUptime", "system_ListPackages", "system_ListServices",
		"system_ListUsers", "system_ListMounts", "system_GetDiskUsage",
		"system_ListProcesses", "system_ListPorts", "system_ListNetworkConnections",
		"system_ReadLogs", "system_GetMetrics",
	}}
}

func knowledgeServiceFunctions() []string {
	return []string{
		"io_Read", "io_List", "io_Stat", "io_ExtractPdf",
		"io_Write", "io_Append", "io_Replace", "io_Remove",
		"document_DetectType", "document_ParseBytes", "document_ExtractText",
		"document_ExtractLayout", "document_GetPages", "document_ExtractTables",
		"document_ExtractImages", "document_RunOCR",
		"runstate_StartRun", "runstate_GetRun", "runstate_ListRuns",
		"runstate_ResumeRun", "runstate_UpdateItemState", "runstate_AppendArtifact",
		"runstate_AppendReference", "runstate_MarkFailed", "runstate_MarkComplete",
		"runstate_CancelRun", "runstate_ListIncompleteItems", "runstate_ListArtifacts",
		"gateway_Embed",
		"indexer_UpsertChunk", "indexer_UpsertFact", "indexer_UpsertEntity",
		"indexer_UpsertRelation", "indexer_UpsertCitation", "indexer_QueryContext",
		"indexer_DeleteDocument", "indexer_DeleteChunk",
		"citation_ResolveSpans", "citation_CreateCitation", "citation_VerifyGrounding",
		"citation_ScoreCoverage", "citation_RenderReferences",
		"core_CreateWorkspaceMutationPlan", "core_ApproveWorkspaceMutationPlan",
		"core_RequestApproval", "core_EvaluatePolicy", "core_RecordAuditEvent",
		"core_PutArtifact",
		"harness_GetContextReport", "harness_StreamContextReports", "harness_PutMemory",
		"harness_GetMemory", "harness_SearchMemory", "harness_DeleteMemory",
	}
}

func devOpsReleaseServiceFunctions() []string {
	return []string{"build_DryRunRelease", "build_InitReleaseConfig", "build_RunRelease"}
}
