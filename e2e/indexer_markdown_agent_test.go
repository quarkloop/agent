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
	"github.com/quarkloop/supervisor/pkg/api"
)

func TestAgentIndexesITCompanyMarkdownDocuments(t *testing.T) {
	indexerAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	embeddingAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
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
	env := utils.StartE2E(t, true, utils.StartOptions{
		WorkingDir: workingDir,
		Embedding:  embedding,
		SupervisorEnv: map[string]string{
			"QUARK_INDEXER_ADDR":   indexerAddr,
			"QUARK_EMBEDDING_ADDR": embeddingAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			dgraphAddr := utils.StartDgraph(t)
			startIndexerServiceAt(t, bins.Indexer, dgraphAddr, indexerAddr)
			startEmbeddingServiceAt(t, bins.Embedding, embeddingAddr, embedding)
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	indexSession, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "it-company-markdown-index",
	})
	if err != nil {
		t.Fatalf("create index session: %v", err)
	}
	utils.WaitForAgentSession(t, env, indexSession.ID, 10*time.Second)

	indexTrace := utils.PostMessageTraceWithOptions(t, ctx, env, indexSession.ID, indexMarkdownDirectoryPrompt(documentsDir, len(documents)), utils.MessageTraceOptions{
		Label:          "index IT company markdown documents",
		OverallTimeout: 6 * time.Minute,
		IdleTimeout:    90 * time.Second,
	})
	utils.Logf(t, "markdown index reply: %s", indexTrace.Text)
	writeArtifact(t, workingDir, "markdown-agent-index-reply.txt", indexTrace.Text)
	writeArtifact(t, workingDir, "markdown-agent-index-tools.txt", strings.Join(indexTrace.ToolStarts, "\n"))
	writeTraceArtifact(t, workingDir, "markdown-agent-index-tool-events.json", indexTrace)

	assertToolStarted(t, indexTrace, "fs")
	assertToolStarted(t, indexTrace, "embedding_Embed")
	assertToolStarted(t, indexTrace, "indexer_IndexDocument")
	assertNoToolErrors(t, indexTrace, "embedding_Embed", "indexer_IndexDocument")
	assertToolSuccessCount(t, indexTrace, "indexer_IndexDocument", len(documents))
	for _, document := range documents {
		if !containsText(indexTrace.Text, document.Filename) {
			t.Fatalf("markdown index confirmation missing filename %q:\n%s", document.Filename, indexTrace.Text)
		}
	}
	verifyPersistedMarkdownIndexState(t, ctx, workingDir, indexerAddr, embeddingAddr, documents)

	querySession, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "it-company-markdown-query",
	})
	if err != nil {
		t.Fatalf("create query session: %v", err)
	}
	utils.WaitForAgentSession(t, env, querySession.ID, 10*time.Second)

	queryTrace := utils.PostMessageTraceWithOptions(t, ctx, env, querySession.ID, indexedMarkdownQuestionPrompt(), utils.MessageTraceOptions{
		Label:          "query IT company markdown index",
		OverallTimeout: 4 * time.Minute,
		IdleTimeout:    90 * time.Second,
	})
	utils.Logf(t, "markdown query reply: %s", queryTrace.Text)
	writeArtifact(t, workingDir, "markdown-agent-query-reply.txt", queryTrace.Text)
	writeArtifact(t, workingDir, "markdown-agent-query-tools.txt", strings.Join(queryTrace.ToolStarts, "\n"))
	writeTraceArtifact(t, workingDir, "markdown-agent-query-tool-events.json", queryTrace)

	assertToolStarted(t, queryTrace, "embedding_Embed")
	assertToolStarted(t, queryTrace, "indexer_GetContext")
	assertNoToolErrors(t, queryTrace, "embedding_Embed", "indexer_GetContext")
	if contains(queryTrace.ToolStarts, "fs") {
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
	assertAnswerContainsAny(t, queryTrace.Text, "4-hour", "Critical incidents")
}
