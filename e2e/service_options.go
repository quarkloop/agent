//go:build e2e

package e2e

import (
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func standardKnowledgeServicesStartOptions(t *testing.T, embedding utils.GatewayEmbeddingOptions, workingDir string) utils.StartOptions {
	t.Helper()
	return utils.StartOptions{
		WorkingDir:              workingDir,
		Embedding:               embedding,
		Agents:                  []string{"quark-main", "quark-knowledge"},
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
			Name:   name,
			Plugin: name,
		})
	}
	return plugins
}

func gatewayServicePlugin() utils.ServicePlugin {
	return utils.ServicePlugin{Name: "gateway", Plugin: "gateway"}
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
	return map[string][]string{
		"quark-main":      knowledgeServiceFunctions(),
		"quark-knowledge": knowledgeServiceFunctions(),
	}
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
		"io_Read", "io_List", "io_Stat",
		"document_ExtractText", "document_GetPages",
		"runstate_StartRun", "runstate_MarkComplete",
		"gateway_Embed",
		"indexer_UpsertChunk", "indexer_QueryContext",
		"citation_VerifyGrounding", "citation_RenderReferences",
	}
}

func devOpsReleaseServiceFunctions() []string {
	return []string{"build_DryRunRelease", "build_InitReleaseConfig", "build_RunRelease"}
}
