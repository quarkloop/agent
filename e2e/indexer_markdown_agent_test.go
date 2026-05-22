//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

func TestAgentIndexesITCompanyMarkdownDocuments(t *testing.T) {
	workingDir := t.TempDir()
	documentsDir := filepath.Join(workingDir, "company-records")
	documents := copyMarkdownDocuments(t, documentsDir, []markdownDocumentFixture{
		{
			Name:   "Aurora cloud migration invoice",
			Source: filepath.Join(utils.QuarkRoot(t), "e2e", "testdata", "it-company-documents", "invoice_2026_aurora_cloud_migration.md"),
			Query:  "invoice INV-2026-014 Northwind Retail Aurora cloud migration",
			Want:   []string{"INV-2026-014", "Northwind Retail GmbH", "EUR 18,450.00"},
		},
		{
			Name:   "Workstation equipment receipt",
			Source: filepath.Join(utils.QuarkRoot(t), "e2e", "testdata", "it-company-documents", "receipt_2026_workstation_equipment.md"),
			Query:  "receipt RCPT-2026-042 ByteWorks workstation equipment total paid",
			Want:   []string{"RCPT-2026-042", "ByteWorks Supply GmbH", "EUR 4,872.65"},
		},
		{
			Name:   "QuarkOps product catalog",
			Source: filepath.Join(utils.QuarkRoot(t), "e2e", "testdata", "it-company-documents", "product_catalog_quark_ops_platform.md"),
			Query:  "QuarkOps Observability Starter SKU monthly price SLA",
			Want:   []string{"QOP-OBS-START", "EUR 1,200.00", "99.9%"},
		},
		{
			Name:   "Acme managed IT support contract",
			Source: filepath.Join(utils.QuarkRoot(t), "e2e", "testdata", "it-company-documents", "support_contract_acme_managed_it.md"),
			Query:  "Acme Manufacturing Sentinel Managed IT contract renewal response target",
			Want:   []string{"MSA-ACME-2026-01", "Sentinel Managed IT", "4-hour"},
		},
	})

	embedding := utils.EmbeddingOptions{
		Plugin:     "embedding",
		Mode:       "local",
		Provider:   "local",
		Model:      "local-hash-v1",
		Dimensions: 32,
	}
	env := utils.StartE2E(t, true, standardKnowledgeServicesStartOptions(t, embedding, workingDir))

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	indexPrompt := indexMarkdownDirectoryPrompt(documentsDir, len(documents))
	indexTrace := runChatPrompt(t, ctx, env, workingDir, chatPromptRun{
		Title:          "it-company-markdown-index",
		Label:          "markdown index",
		ArtifactPrefix: "markdown-agent-index",
		Prompt:         indexPrompt,
		TraceOptions:   knowledgeIndexTraceOptions("index IT company markdown documents", len(documents)),
	})

	assertToolStarted(t, indexTrace, "ingestion_StartRun")
	assertToolStarted(t, indexTrace, "io_Read")
	assertToolStarted(t, indexTrace, "embedding_Embed")
	assertToolStarted(t, indexTrace, "indexer_UpsertChunk")
	assertToolStarted(t, indexTrace, "ingestion_MarkComplete")
	assertNoToolErrors(t, indexTrace, "indexer_UpsertChunk")
	assertEmbeddingSuccessCount(t, indexTrace, len(documents))
	assertToolSuccessCount(t, indexTrace, "indexer_UpsertChunk", len(documents))
	for _, document := range documents {
		if !containsText(indexTrace.Text, document.Filename) {
			t.Fatalf("markdown index confirmation missing filename %q:\n%s", document.Filename, indexTrace.Text)
		}
	}
	verifyPersistedMarkdownIndexState(t, ctx, workingDir, env.ServiceAddress("indexer"), env.ServiceAddress("embedding"), documents)

	queryPrompt := indexedMarkdownQuestionPrompt()
	queryTrace := runChatPrompt(t, ctx, env, workingDir, chatPromptRun{
		Title:          "it-company-markdown-query",
		Label:          "markdown query",
		ArtifactPrefix: "markdown-agent-query",
		Prompt:         queryPrompt,
		TraceOptions:   knowledgeQueryTraceOptions("query IT company markdown index"),
	})

	assertToolStarted(t, queryTrace, "embedding_Embed")
	assertToolStarted(t, queryTrace, "indexer_QueryContext")
	assertToolStartedAny(t, queryTrace, "citation_VerifyGrounding", "citation_RenderReferences")
	assertNoToolErrors(t, queryTrace, "embedding_Embed", "indexer_QueryContext", "citation_VerifyGrounding", "citation_RenderReferences")
	if contains(queryTrace.ToolStarts, "io_Read") || contains(queryTrace.ToolStarts, "io_List") {
		t.Fatalf("markdown query re-read source files instead of using the index; starts=%v", queryTrace.ToolStarts)
	}
	assertAnswerContains(t, queryTrace.Text,
		"INV-2026-014",
		"Northwind Retail GmbH",
		"RCPT-2026-042",
		"EUR 4,872.65",
		"QOP-OBS-START",
		"Sentinel Managed IT",
	)
	assertAnswerContainsAny(t, queryTrace.Text, "4-hour", "4 hours", "Critical incident", "Critical-incident")
}
