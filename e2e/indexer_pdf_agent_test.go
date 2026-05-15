//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/supervisor/pkg/api"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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

	indexerAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	embeddingAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
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

	indexTrace := utils.PostMessageTraceWithOptions(t, ctx, env, indexSession.ID, indexPDFDocumentsPrompt(documents), utils.MessageTraceOptions{
		Label:          "index uploaded PDF dataset",
		OverallTimeout: 8 * time.Minute,
		IdleTimeout:    90 * time.Second,
	})
	utils.Logf(t, "index reply: %s", indexTrace.Text)
	writeArtifact(t, workingDir, "agent-index-reply.txt", indexTrace.Text)
	writeArtifact(t, workingDir, "agent-index-tools.txt", strings.Join(indexTrace.ToolStarts, "\n"))
	writeTraceArtifact(t, workingDir, "agent-index-tool-events.json", indexTrace)

	assertToolStarted(t, indexTrace, "fs")
	assertToolStarted(t, indexTrace, "embedding_Embed")
	assertToolStarted(t, indexTrace, "indexer_IndexDocument")
	assertNoToolErrors(t, indexTrace, "embedding_Embed", "indexer_IndexDocument")
	assertToolSuccessCount(t, indexTrace, "indexer_IndexDocument", len(documents))
	assertAgentStructuredPDFIndexPayloads(t, indexTrace, documents)
	for _, document := range documents {
		if !containsText(indexTrace.Text, document.Filename) {
			t.Fatalf("index confirmation missing filename %q:\n%s", document.Filename, indexTrace.Text)
		}
	}
	verifyPersistedPDFIndexState(t, ctx, workingDir, indexerAddr, embeddingAddr, documents)

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

		queryTrace := utils.PostMessageTraceWithOptions(t, ctx, env, querySession.ID, indexedPDFQuestionPrompt(queryCase.Question, len(documents)), utils.MessageTraceOptions{
			Label:          "query indexed PDF dataset: " + queryCase.Title,
			OverallTimeout: 4 * time.Minute,
			IdleTimeout:    90 * time.Second,
		})
		utils.Logf(t, "%s query reply: %s", queryCase.Title, queryTrace.Text)
		artifactPrefix := "agent-query-" + queryCase.Title
		writeArtifact(t, workingDir, artifactPrefix+"-reply.txt", queryTrace.Text)
		writeArtifact(t, workingDir, artifactPrefix+"-tools.txt", strings.Join(queryTrace.ToolStarts, "\n"))
		writeTraceArtifact(t, workingDir, artifactPrefix+"-tool-events.json", queryTrace)

		assertToolStarted(t, queryTrace, "embedding_Embed")
		assertToolStarted(t, queryTrace, "indexer_GetContext")
		assertNoToolErrors(t, queryTrace, "embedding_Embed", "indexer_GetContext")
		if contains(queryTrace.ToolStarts, "fs") {
			t.Fatalf("%s query re-read source files instead of using the index; starts=%v", queryCase.Title, queryTrace.ToolStarts)
		}
		assertToolResultContains(t, queryTrace, "indexer_GetContext", "reasoningContext")
		assertToolResultContains(t, queryTrace, "indexer_GetContext", "embeddingMetadata", env.Embedding.Provider, env.Embedding.Model, fmt.Sprint(env.Embedding.Dimensions))
		assertAnswerContains(t, queryTrace.Text, queryCase.Want...)
		if len(queryCase.WantAny) > 0 {
			assertAnswerContainsAny(t, queryTrace.Text, queryCase.WantAny...)
		}
	}
}

func verifyPersistedPDFIndexState(t *testing.T, ctx context.Context, artifactDir, indexerAddr, embeddingAddr string, documents []indexedPDFDocument) {
	t.Helper()
	embeddingConn, err := servicekit.Dial(ctx, embeddingAddr)
	if err != nil {
		t.Fatalf("dial embedding service for persisted-state verification: %v", err)
	}
	defer embeddingConn.Close()
	indexerConn, err := servicekit.Dial(ctx, indexerAddr)
	if err != nil {
		t.Fatalf("dial indexer service for persisted-state verification: %v", err)
	}
	defer indexerConn.Close()

	embeddingClient := embeddingv1.NewEmbeddingServiceClient(embeddingConn)
	indexerClient := indexerv1.NewIndexerServiceClient(indexerConn)
	report := make([]map[string]any, 0, len(documents))
	for _, document := range documents {
		embedding, err := embeddingClient.Embed(ctx, &embeddingv1.EmbedRequest{
			Input: document.Name + " " + document.Filename,
		})
		if err != nil {
			t.Fatalf("embed verification query for %s: %v", document.Filename, err)
		}
		resp, err := indexerClient.GetContext(ctx, &indexerv1.QueryRequest{
			QueryVector: embedding.GetVector(),
			Limit:       1,
			Depth:       1,
			Filters:     map[string]string{"filename": document.Filename},
		})
		if err != nil {
			t.Fatalf("query persisted index state for %s: %v", document.Filename, err)
		}
		if len(resp.GetChunks()) == 0 {
			t.Fatalf("persisted index state missing chunk for %s", document.Filename)
		}
		chunk := resp.GetChunks()[0]
		if got := chunk.GetMetadata()["filename"]; got != document.Filename {
			t.Fatalf("persisted chunk filename = %q, want %q: %+v", got, document.Filename, chunk.GetMetadata())
		}
		if chunk.GetEmbeddingMetadata().GetDimensions() != embedding.GetDimensions() {
			t.Fatalf("persisted embedding dimensions for %s = %d, want %d", document.Filename, chunk.GetEmbeddingMetadata().GetDimensions(), embedding.GetDimensions())
		}
		report = append(report, map[string]any{
			"filename":           document.Filename,
			"chunk_id":           chunk.GetId(),
			"score":              chunk.GetScore(),
			"embedding_provider": chunk.GetEmbeddingMetadata().GetProvider(),
			"embedding_model":    chunk.GetEmbeddingMetadata().GetModel(),
			"embedding_dims":     chunk.GetEmbeddingMetadata().GetDimensions(),
			"citations":          resp.GetCitations(),
			"context_confidence": resp.GetContextPackage().GetConfidence(),
		})
	}
	writeJSONArtifact(t, artifactDir, "direct-index-state.json", report)
}

func startEmbeddingServiceAt(t *testing.T, binary, addr string, embedding utils.EmbeddingOptions) {
	t.Helper()
	embedding = embedding.WithDefaults()
	utils.StartProcess(t, "embedding", binary, []string{
		"--addr", addr,
		"--skill-dir", filepath.Join(utils.QuarkRoot(t), "plugins", "services", embedding.Plugin),
		"--provider", embedding.Provider,
		"--model", embedding.Model,
		"--dimensions", fmt.Sprint(embedding.Dimensions),
	}, utils.ProcessEnv(nil))

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, err := servicekit.Dial(ctx, addr)
		if err == nil {
			_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{
				Service: embeddingv1.EmbeddingService_ServiceDesc.ServiceName,
			})
			conn.Close()
		}
		cancel()
		if err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("embedding service did not become healthy at %s", addr)
}

type pdfDocumentFixture struct {
	Name   string
	Source string
}

type indexedPDFDocument struct {
	Name     string
	Path     string
	Filename string
}

type indexedPDFQueryCase struct {
	Title    string
	Question string
	Want     []string
	WantAny  []string
}

func copyPDFDocuments(t *testing.T, dir string, fixtures []pdfDocumentFixture) []indexedPDFDocument {
	t.Helper()
	out := make([]indexedPDFDocument, 0, len(fixtures))
	for _, fixture := range fixtures {
		filename := filepath.Base(fixture.Source)
		dst := filepath.Join(dir, filename)
		copyTestFile(t, fixture.Source, dst)
		out = append(out, indexedPDFDocument{
			Name:     fixture.Name,
			Path:     dst,
			Filename: filename,
		})
	}
	return out
}

func indexPDFDocumentsPrompt(documents []indexedPDFDocument) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Please index this uploaded PDF dataset for later search and Q&A. Treat these %d files as one user upload batch.\n\n", len(documents))
	b.WriteString("Documents:\n")
	for _, document := range documents {
		fmt.Fprintf(&b, "- %s (%s)\n", document.Path, document.Name)
	}
	fmt.Fprintf(&b, "\nRequired workflow: read each uploaded PDF through the available file/document extraction functions, then perform semantic extraction yourself through the runtime LLM context before indexing. Do not ask the user or the test for IDs, entities, facts, citations, or payload data. For every listed file, create one compact searchable chunk and agent-produced structured knowledge: document metadata, facts, entities, relations, citations, and provenance tied to source text. Call embedding_Embed without setting dimensions for that chunk, then immediately call indexer_IndexDocument for the same file using the returned embeddingRef. Each IndexDocument call must include first-class document, textContent, embeddingRef, facts, entities, relations, citations, provenance, and sourceMetadata with filename and path. Do not rename files, restructure directories, write sidecars, or finish until there are %d successful indexer_IndexDocument results, one for each listed file. Reply briefly with the filenames that are ready for questions.", len(documents))
	return b.String()
}

func indexedPDFQuestionPrompt(question string, documentCount int) string {
	limit := documentCount * 2
	if limit < 8 {
		limit = 8
	}
	return fmt.Sprintf(`Search the indexed PDFs and answer this question from the indexed context:

%s

Do not re-read the source PDFs. First embed this question with embedding_Embed. Then call indexer_GetContext with queryVectorRef, limit %d, and depth 1. Use the returned reasoningContext only, and include the source filename when available.`, question, limit)
}

func copyTestFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func writeArtifact(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write artifact %s: %v", path, err)
	}
	utils.Logf(t, "manual verification artifact: %s", path)
}

func writeTraceArtifact(t *testing.T, dir, name string, trace utils.MessageTrace) {
	t.Helper()
	payload := map[string]any{
		"text":    trace.Text,
		"starts":  trace.ToolStartEvents,
		"results": trace.ToolResultEvents,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal trace artifact: %v", err)
	}
	writeArtifact(t, dir, name, string(data))
}

func writeJSONArtifact(t *testing.T, dir, name string, payload any) {
	t.Helper()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal artifact %s: %v", name, err)
	}
	writeArtifact(t, dir, name, string(data))
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsText(value, want string) bool {
	return strings.Contains(strings.ToLower(canonicalText(value)), strings.ToLower(canonicalText(want)))
}

func containsAnyText(value string, wants ...string) bool {
	for _, want := range wants {
		if containsText(value, want) {
			return true
		}
	}
	return false
}

func canonicalText(value string) string {
	replacer := strings.NewReplacer(
		"\u00ad", "",
		"\u2010", "-",
		"\u2011", "-",
		"\u2012", "-",
		"\u2013", "-",
		"\u2014", "-",
		"\u2212", "-",
		"\u00a0", " ",
		"\u2007", " ",
		"\u2009", " ",
		"\u202f", " ",
	)
	return replacer.Replace(value)
}

func assertToolStarted(t *testing.T, trace utils.MessageTrace, name string) {
	t.Helper()
	if !contains(trace.ToolStarts, name) {
		t.Fatalf("agent did not start %s tool; starts=%v", name, trace.ToolStarts)
	}
}

func assertToolResultContains(t *testing.T, trace utils.MessageTrace, tool string, wants ...string) {
	t.Helper()
	for _, event := range trace.ToolResultEvents {
		if event.Name != tool {
			continue
		}
		missing := false
		for _, want := range wants {
			if !containsText(event.Result, want) {
				missing = true
				break
			}
		}
		if !missing {
			return
		}
	}
	t.Fatalf("%s tool results missing %v: %+v", tool, wants, trace.ToolResultEvents)
}

func assertNoToolErrors(t *testing.T, trace utils.MessageTrace, tools ...string) {
	t.Helper()
	wanted := make(map[string]bool, len(tools))
	for _, tool := range tools {
		wanted[tool] = true
	}
	for _, event := range trace.ToolResultEvents {
		if !wanted[event.Name] {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(event.Result), "error:") {
			t.Fatalf("%s returned an error: %s", event.Name, event.Result)
		}
	}
}

func assertToolSuccessCount(t *testing.T, trace utils.MessageTrace, tool string, want int) {
	t.Helper()
	count := 0
	for _, event := range trace.ToolResultEvents {
		if event.Name != tool {
			continue
		}
		compact := strings.ReplaceAll(event.Result, " ", "")
		if strings.Contains(compact, `"success":true`) {
			count++
		}
	}
	if count < want {
		t.Fatalf("%s successful results = %d, want at least %d: %+v", tool, count, want, trace.ToolResultEvents)
	}
}

func assertAgentStructuredPDFIndexPayloads(t *testing.T, trace utils.MessageTrace, documents []indexedPDFDocument) {
	t.Helper()
	payloads := indexerIndexDocumentPayloads(t, trace)
	if len(payloads) < len(documents) {
		t.Fatalf("indexer_IndexDocument payload count = %d, want at least %d", len(payloads), len(documents))
	}

	seen := make(map[string]bool, len(documents))
	for i, payload := range payloads {
		if !hasNonEmptyString(payload, "chunkId") {
			t.Fatalf("IndexDocument payload %d missing chunkId: %+v", i, payload)
		}
		if !hasNonEmptyString(payload, "textContent") {
			t.Fatalf("IndexDocument payload %d missing textContent: %+v", i, payload)
		}
		if !hasNonEmptyString(payload, "embeddingRef") {
			t.Fatalf("IndexDocument payload %d missing embeddingRef from embedding_Embed: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "document") {
			t.Fatalf("IndexDocument payload %d missing first-class document metadata: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "sourceMetadata") {
			t.Fatalf("IndexDocument payload %d missing sourceMetadata: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "facts") {
			t.Fatalf("IndexDocument payload %d missing agent-produced facts: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "entities") {
			t.Fatalf("IndexDocument payload %d missing agent-produced entities: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "relations") {
			t.Fatalf("IndexDocument payload %d missing agent-produced relations: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "citations") {
			t.Fatalf("IndexDocument payload %d missing source citations: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "provenance") {
			t.Fatalf("IndexDocument payload %d missing provenance: %+v", i, payload)
		}
		for _, document := range documents {
			if payloadMentionsFilename(payload, document.Filename) {
				seen[document.Filename] = true
			}
		}
	}
	for _, document := range documents {
		if !seen[document.Filename] {
			t.Fatalf("no structured IndexDocument payload referenced %s: %+v", document.Filename, payloads)
		}
	}
}

func indexerIndexDocumentPayloads(t *testing.T, trace utils.MessageTrace) []map[string]any {
	t.Helper()
	payloads := make([]map[string]any, 0)
	for _, event := range trace.ToolStartEvents {
		if event.Name != "indexer_IndexDocument" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Arguments), &payload); err != nil {
			t.Fatalf("decode indexer_IndexDocument arguments: %v\n%s", err, event.Arguments)
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

func payloadMentionsFilename(payload map[string]any, filename string) bool {
	if containsText(fmt.Sprint(payload["sourceMetadata"]), filename) {
		return true
	}
	if containsText(fmt.Sprint(payload["document"]), filename) {
		return true
	}
	if containsText(fmt.Sprint(payload["provenance"]), filename) {
		return true
	}
	return false
}

func hasNonEmptyString(payload map[string]any, key string) bool {
	value, ok := payload[key].(string)
	return ok && strings.TrimSpace(value) != ""
}

func hasNonEmptyObject(payload map[string]any, key string) bool {
	value, ok := payload[key].(map[string]any)
	return ok && len(value) > 0
}

func hasNonEmptyArray(payload map[string]any, key string) bool {
	value, ok := payload[key].([]any)
	return ok && len(value) > 0
}

func assertAnswerContains(t *testing.T, answer string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !containsText(answer, want) {
			t.Fatalf("answer missing %q:\n%s", want, answer)
		}
	}
}

func assertAnswerContainsAny(t *testing.T, answer string, wants ...string) {
	t.Helper()
	if !containsAnyText(answer, wants...) {
		t.Fatalf("answer missing one of %v:\n%s", wants, answer)
	}
}
