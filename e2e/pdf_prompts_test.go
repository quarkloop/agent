//go:build e2e

package e2e

import (
	"fmt"
	"strings"
)

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
