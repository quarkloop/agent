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
	fmt.Fprintf(&b, "\nRequired workflow: call fs extract_pdf for each uploaded PDF, then perform semantic extraction yourself through the runtime LLM context before indexing. The fs extract_pdf result includes contentRef for the exact extracted PDF text. Do not ask the user or the test for IDs, entities, facts, citations, or payload data. For every listed file, create one compact searchable chunk by passing that contentRef as textContentRef to indexer_IndexDocument; do not copy, paraphrase, abbreviate, correct, or infer the source text into textContent. Also include agent-produced structured knowledge for every file: document metadata, at least two concrete facts, at least two named entities, citations tied to source text, provenance, and a relations array (empty only if no relation is supported by the document). Call embedding_Embed with the same contentRef as inputRef or contentRef and without setting dimensions, then immediately call indexer_IndexDocument for the same file using the returned embeddingRef and textContentRef. Never pass a filesystem path, filename, document title, or extracted text as inputRef, contentRef, textContentRef, embeddingRef, or queryVectorRef; every *Ref value must be copied exactly from a previous tool result in this same session. Use at most one service function call per assistant turn, and every service function argument payload must be one valid JSON object with quoted keys and schema-compatible values. Each IndexDocument call must include first-class document, textContentRef, embeddingRef, facts, entities, relations, citations, provenance, and sourceMetadata with filename, path, source_content_ref, embedding_provider, embedding_model, and embedding_dimensions. Do not rename files, restructure directories, write sidecars, or finish until there are %d successful indexer_IndexDocument results, one for each listed file. Reply briefly with the filenames that are ready for questions.", len(documents))
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
