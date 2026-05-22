//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

func TestAgentIndexesUploadedPDFDataset(t *testing.T) {
	runAgentIndexesUploadedPDFDataset(t, utils.EmbeddingOptions{
		Plugin:     "embedding",
		Mode:       "local",
		Provider:   "local",
		Model:      "local-hash-v1",
		Dimensions: 32,
	})
}

func TestAgentIndexesUploadedPDFDatasetOpenRouterEmbedding(t *testing.T) {
	model := strings.TrimSpace(os.Getenv("OPENROUTER_E2E_EMBEDDING_MODEL"))
	if model == "" {
		t.Skip("set OPENROUTER_E2E_EMBEDDING_MODEL to run OpenRouter embedding e2e coverage")
	}
	runAgentIndexesUploadedPDFDataset(t, utils.EmbeddingOptions{
		Plugin:     "embedding-openrouter",
		Mode:       "online",
		Provider:   "openrouter",
		Model:      model,
		Dimensions: 2048,
	})
}

func runAgentIndexesUploadedPDFDataset(t *testing.T, embedding utils.EmbeddingOptions) {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is required by the document service PDF backend")
	}

	workingDir := t.TempDir()
	documents := copyPDFDocuments(t, workingDir, []pdfDocumentFixture{
		{
			Name:   "AI mini-app idea catalog",
			Source: filepath.Join(utils.QuarkRoot(t), "e2e", "testdata", "documents", "mini_app_ideas_catalog.pdf"),
		},
		{
			Name:   "Transformer research paper",
			Source: filepath.Join(utils.QuarkRoot(t), "e2e", "testdata", "documents", "attention_is_all_you_need_paper.pdf"),
		},
		{
			Name:   "Europass resume sample",
			Source: filepath.Join(utils.QuarkRoot(t), "e2e", "testdata", "documents", "europass_resume_sample.pdf"),
		},
		{
			Name:   "German health insurance information sheet",
			Source: filepath.Join(utils.QuarkRoot(t), "e2e", "testdata", "documents", "german_health_insurance_information_sheet.pdf"),
		},
	})

	env := utils.StartE2E(t, true, standardKnowledgeServicesStartOptions(t, embedding, workingDir))
	writeJSONArtifact(t, workingDir, "embedding-profile.json", map[string]any{
		"plugin":     env.Embedding.Plugin,
		"mode":       env.Embedding.Mode,
		"provider":   env.Embedding.Provider,
		"model":      env.Embedding.Model,
		"dimensions": env.Embedding.Dimensions,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	indexPrompt := indexPDFDocumentsPrompt(documents)
	indexTrace := runChatPrompt(t, ctx, env, workingDir, chatPromptRun{
		Title:          "pdf-indexer-agent-test",
		Label:          "index",
		ArtifactPrefix: "agent-index",
		Prompt:         indexPrompt,
		TraceOptions:   knowledgeIndexTraceOptions("index uploaded PDF dataset", len(documents)),
	})

	assertToolStarted(t, indexTrace, "ingestion_StartRun")
	assertToolStarted(t, indexTrace, "document_ExtractText")
	assertToolStarted(t, indexTrace, "embedding_Embed")
	assertToolStarted(t, indexTrace, "indexer_UpsertChunk")
	assertToolStarted(t, indexTrace, "ingestion_MarkComplete")
	assertNoToolErrors(t, indexTrace, "document_ExtractText")
	assertToolSuccessCount(t, indexTrace, "document_ExtractText", len(documents))
	assertNoToolErrors(t, indexTrace, "indexer_UpsertChunk")
	assertEmbeddingSuccessCount(t, indexTrace, len(documents))
	assertToolSuccessCount(t, indexTrace, "indexer_UpsertChunk", len(documents))
	assertAgentStructuredPDFIndexPayloads(t, indexTrace, documents)
	for _, document := range documents {
		if !containsText(indexTrace.Text, document.Filename) {
			t.Fatalf("index confirmation missing filename %q:\n%s", document.Filename, indexTrace.Text)
		}
	}
	verifyPersistedPDFIndexState(t, ctx, workingDir, env.ServiceAddress("indexer"), env.ServiceAddress("embedding"), documents)

	queryCases := []indexedPDFQueryCase{{
		Title:    "dataset-summary",
		Question: "Across the indexed PDFs, identify: the Transformer architecture paper and its core idea; the resume candidate and senior role; the PDF about health insurance requirements for residence permits; and the PDF that lists AI mini-app ideas with one category or app idea.",
		Want: []string{
			"attention_is_all_you_need_paper.pdf",
			"Transformer",
			"europass_resume_sample.pdf",
			"John Doe",
			"Senior Software Engineer",
			"german_health_insurance_information_sheet.pdf",
			"mini_app_ideas_catalog.pdf",
		},
		WantAny: []string{"attention", "self-attention", "health insurance", "residence permit", "Productivity", "AI Business Productivity Assistant", "mini-app"},
	}}
	for _, queryCase := range queryCases {
		queryPrompt := indexedPDFQuestionPrompt(queryCase.Question)
		artifactPrefix := "agent-query-" + queryCase.Title
		queryTrace := runChatPrompt(t, ctx, env, workingDir, chatPromptRun{
			Title:          "pdf-indexer-query-" + queryCase.Title,
			Label:          queryCase.Title + " query",
			ArtifactPrefix: artifactPrefix,
			Prompt:         queryPrompt,
			TraceOptions:   knowledgeQueryTraceOptions("query indexed PDF dataset: " + queryCase.Title),
		})

		assertToolStarted(t, queryTrace, "embedding_Embed")
		assertToolStarted(t, queryTrace, "indexer_QueryContext")
		assertToolStartedAny(t, queryTrace, "citation_VerifyGrounding", "citation_RenderReferences")
		assertNoToolErrors(t, queryTrace, "embedding_Embed", "indexer_QueryContext", "citation_VerifyGrounding", "citation_RenderReferences")
		if contains(queryTrace.ToolStarts, "io_Read") || contains(queryTrace.ToolStarts, "io_ExtractPdf") {
			t.Fatalf("%s query re-read source files instead of using the index; starts=%v", queryCase.Title, queryTrace.ToolStarts)
		}
		assertIndexerQueryReturnedStructuredContext(t, queryTrace)
		assertEmbeddingToolResult(t, queryTrace, env.Embedding.Provider, env.Embedding.Model, env.Embedding.Dimensions)
		assertAnswerContains(t, queryTrace.Text, queryCase.Want...)
		if len(queryCase.WantAny) > 0 {
			assertAnswerContainsAny(t, queryTrace.Text, queryCase.WantAny...)
		}
	}
}
