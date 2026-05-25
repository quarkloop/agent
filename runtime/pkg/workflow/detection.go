package workflow

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/quarkloop/pkg/plugin"
)

// Detect maps a user request and the resolved callable surface to required
// workflow steps; unavailable service functions are never advertised.
func Detect(prompt string, tools []plugin.ToolSchema) []Intent {
	available := availableTools(tools)
	text := normalize(prompt)
	intents := make([]Intent, 0, 2)
	if looksLikeKnowledgeIndex(text) {
		sourceCount := expectedKnowledgeSourceCount(text)
		if steps := requiredSteps(available,
			step("ingest-start", "durable run creation", "runstate_StartRun"),
			stepCount("extract", "document content extraction", sourceCount, "document_ExtractText", "document_GetPages"),
			stepCount("embed", "embedding generation", sourceCount, "gateway_Embed", "gateway_Embed"),
			stepCount("index", "canonical indexing", sourceCount, "indexer_UpsertChunk"),
			step("ingest-complete", "durable run completion", "runstate_MarkComplete"),
		); len(steps) > 0 {
			intents = append(intents, Intent{Kind: KindKnowledgeIndex, Steps: steps})
		}
	}
	if looksLikeKnowledgeQuery(text) {
		if steps := requiredSteps(available,
			step("embed-query", "query embedding", "gateway_Embed", "gateway_Embed"),
			step("retrieve", "context retrieval", "indexer_QueryContext", "indexer_GetContext"),
			step("ground", "grounding or citation verification", "citation_VerifyGrounding", "citation_RenderReferences"),
		); len(steps) > 0 {
			intents = append(intents, Intent{Kind: KindKnowledgeQuery, Steps: steps})
		}
	}
	if steps := devopsSteps(text, available); len(steps) > 0 {
		intents = append(intents, Intent{Kind: KindDevOps, Steps: steps})
	}
	if steps := systemSteps(text, available); len(steps) > 0 {
		kind := KindSystemInspect
		if containsAny(text, " kill ", " restart ", " stop ", " terminate ") {
			kind = KindSystemMutation
		}
		intents = append(intents, Intent{Kind: kind, Steps: steps})
	}
	return intents
}

func availableTools(tools []plugin.ToolSchema) map[string]struct{} {
	available := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) != "" {
			available[tool.Name] = struct{}{}
		}
	}
	return available
}

func requiredSteps(available map[string]struct{}, specs ...Step) []Step {
	steps := make([]Step, 0, len(specs))
	for _, spec := range specs {
		filtered := make([]string, 0, len(spec.AnyOf))
		for _, name := range spec.AnyOf {
			if _, ok := available[name]; ok {
				filtered = append(filtered, name)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		spec.AnyOf = filtered
		steps = append(steps, spec)
	}
	return steps
}

func devopsSteps(text string, available map[string]struct{}) []Step {
	specs := make([]Step, 0)
	releaseRequested := containsAny(text, " release ", " publish ", " tag ")
	genericBuildRequested := containsAny(text, " build ", " compile ", " package ")
	if releaseRequested && containsAny(text, " dry run ", " dryrun ", " preview ", " plan ", " without publishing ") {
		genericBuildRequested = false
	}
	if containsAny(text, " repo ", " repository ", " git ", " status ", " changed ", " diff ") {
		specs = append(specs, step("repo-status", "repository inspection", "repo_Status", "repo_ListChangedFiles", "repo_Diff"))
	}
	if containsAny(text, " project ", " project kind ", " detect project ", " go project ", " package ") {
		specs = append(specs, step("project-detect", "project detection", "build_DetectProject"))
	}
	if containsAny(text, " test ", " tests ", " failing ", " failure ") {
		specs = append(specs, step("tests", "test execution or discovery", "test_RunTests", "test_DiscoverTests"))
	}
	if containsAny(text, " explain ", " failure ", " failing ") {
		specs = append(specs, step("explain-failure", "failure explanation", "test_ExplainFailure"))
	}
	if genericBuildRequested {
		specs = append(specs, step("build", "build execution", "build_RunTask", "build_CreateArtifact"))
	}
	if releaseRequested {
		specs = append(specs, step("release", "release planning", "build_DryRunRelease", "repo_GenerateReleaseNotes"))
	}
	if containsAny(text, " release ", " publish ", " deploy ", " apply ", " patch ", " commit ") {
		specs = append(specs, step("policy", "policy evaluation", "policy_EvaluateChange"))
	}
	return requiredSteps(available, specs...)
}

func systemSteps(text string, available map[string]struct{}) []Step {
	specs := make([]Step, 0)
	if containsAny(text, " snapshot ", " system ", " machine ", " host ") {
		specs = append(specs, step("snapshot", "system snapshot", "system_Snapshot"))
	}
	if containsAny(text, " process ", " processes ", " pid ", " kill ", " terminate ") {
		specs = append(specs, step("processes", "process inspection", "system_ListProcesses", "system_KillProcess"))
	}
	if containsAny(text, " port ", " ports ", " network ", " socket ", " connection ") {
		specs = append(specs, step("network", "network inspection", "system_ListPorts", "system_ListNetworkConnections"))
	}
	if containsAny(text, " disk ", " mount ", " filesystem ", " storage ") {
		specs = append(specs, step("disk", "disk or mount inspection", "system_GetDiskUsage", "system_ListMounts"))
	}
	if containsAny(text, " log ", " logs ", " journal ") {
		specs = append(specs, step("logs", "log inspection", "system_ReadLogs"))
	}
	if containsAny(text, " metric ", " metrics ", " memory ", " load ", " cpu ") {
		specs = append(specs, step("metrics", "system metrics", "system_GetMetrics"))
	}
	if containsAny(text, " service ", " services ", " restart ") {
		specs = append(specs, step("services", "service inspection or restart plan", "system_ListServices", "system_RestartService"))
	}
	if containsAny(text, " package ", " packages ", " installed ") {
		specs = append(specs, step("packages", "package inventory", "system_ListPackages"))
	}
	return requiredSteps(available, specs...)
}

func step(id, label string, anyOf ...string) Step {
	return stepCount(id, label, 1, anyOf...)
}

func stepCount(id, label string, count int, anyOf ...string) Step {
	if count <= 0 {
		count = 1
	}
	return Step{ID: id, Label: label, RequiredCount: count, AnyOf: append([]string(nil), anyOf...)}
}

var nonWord = regexp.MustCompile(`[^a-z0-9_/-]+`)

func normalize(input string) string {
	lower := strings.ToLower(input)
	return " " + strings.TrimSpace(nonWord.ReplaceAllString(lower, " ")) + " "
}

func looksLikeKnowledgeIndex(text string) bool {
	return containsAny(text, " index ", " ingest ", " add ") &&
		containsAny(text, " file ", " files ", " pdf ", " document ", " documents ", " directory ", " folder ", " markdown ")
}

func looksLikeKnowledgeQuery(text string) bool {
	if looksLikeKnowledgeIndex(text) {
		return false
	}
	return containsAny(text, " what ", " who ", " when ", " where ", " why ", " how ", " query ", " search ", " find ", " answer ", " summarize ") &&
		containsAny(text, " document ", " documents ", " index ", " indexed ", " knowledge ", " source ", " sources ", " pdf ")
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func expectedKnowledgeSourceCount(text string) int {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\ball\s+(\d{1,3})\s+(?:documents?|files?|pdfs?|records?|markdown)\b`),
		regexp.MustCompile(`\b(\d{1,3})\s+(?:documents?|files?|pdfs?|records?|markdown)\b`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) != 2 {
			continue
		}
		var count int
		if _, err := fmt.Sscanf(match[1], "%d", &count); err == nil && count > 0 {
			return count
		}
	}
	return 1
}
