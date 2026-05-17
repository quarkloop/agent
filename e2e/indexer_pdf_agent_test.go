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
	"github.com/quarkloop/supervisor/pkg/api"
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
		t.Skip("pdftotext is required by the fs extract_pdf tool")
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

	indexSession, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "pdf-indexer-agent-test",
	})
	if err != nil {
		t.Fatalf("create index session: %v", err)
	}
	utils.WaitForAgentSession(t, env, indexSession.ID, 10*time.Second)

	indexPrompt := indexPDFDocumentsPrompt(documents)
	indexTrace := utils.PostMessageTraceWithOptions(t, ctx, env, indexSession.ID, indexPrompt, utils.MessageTraceOptions{
		Label:          "index uploaded PDF dataset",
		OverallTimeout: 8 * time.Minute,
		IdleTimeout:    90 * time.Second,
	})
	utils.Logf(t, "index reply: %s", indexTrace.Text)
	writeAgentRunArtifacts(t, workingDir, "agent-index", env, indexTrace, indexPrompt)

	assertToolStarted(t, indexTrace, "fs")
	assertToolStarted(t, indexTrace, "embedding_Embed")
	assertToolStarted(t, indexTrace, "indexer_IndexDocument")
	assertNoToolErrors(t, indexTrace, "indexer_IndexDocument")
	assertEmbeddingSuccessCount(t, indexTrace, len(documents))
	assertToolSuccessCount(t, indexTrace, "indexer_IndexDocument", len(documents))
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
		querySession, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
			Type:  api.SessionTypeChat,
			Title: "pdf-indexer-query-" + queryCase.Title,
		})
		if err != nil {
			t.Fatalf("create query session %s: %v", queryCase.Title, err)
		}
		utils.WaitForAgentSession(t, env, querySession.ID, 10*time.Second)

		queryPrompt := indexedPDFQuestionPrompt(queryCase.Question)
		queryTrace := utils.PostMessageTraceWithOptions(t, ctx, env, querySession.ID, queryPrompt, utils.MessageTraceOptions{
			Label:          "query indexed PDF dataset: " + queryCase.Title,
			OverallTimeout: 4 * time.Minute,
			IdleTimeout:    90 * time.Second,
		})
		utils.Logf(t, "%s query reply: %s", queryCase.Title, queryTrace.Text)
		artifactPrefix := "agent-query-" + queryCase.Title
		writeAgentRunArtifacts(t, workingDir, artifactPrefix, env, queryTrace, queryPrompt)

		assertToolStarted(t, queryTrace, "embedding_Embed")
		assertToolStarted(t, queryTrace, "indexer_GetContext")
		assertNoToolErrors(t, queryTrace, "embedding_Embed", "indexer_GetContext")
		if contains(queryTrace.ToolStarts, "fs") {
			t.Fatalf("%s query re-read source files instead of using the index; starts=%v", queryCase.Title, queryTrace.ToolStarts)
		}
		assertToolResultContains(t, queryTrace, "indexer_GetContext", "reasoningContext")
		assertEmbeddingToolResult(t, queryTrace, env.Embedding.Provider, env.Embedding.Model, env.Embedding.Dimensions)
		assertAnswerContains(t, queryTrace.Text, queryCase.Want...)
		if len(queryCase.WantAny) > 0 {
			assertAnswerContainsAny(t, queryTrace.Text, queryCase.WantAny...)
		}
	}
}
