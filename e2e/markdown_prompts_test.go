//go:build e2e

package e2e

import "fmt"

func indexMarkdownDirectoryPrompt(directory string, documentCount int) string {
	return fmt.Sprintf(`Please index the Markdown documents in this company records directory for later structured Q&A:

%s

Treat every .md file under that directory as one user upload batch. Required workflow: first call fs list on the directory with recursive=true and include_hash=true. Then read every discovered .md file with fs read. The fs read result includes contentRef for the exact file text. For each Markdown file, create one searchable chunk by passing that contentRef as textContentRef to indexer_IndexDocument; do not copy, paraphrase, abbreviate, correct, or infer the source text into textContent. Use facts, entities, relations, citations, provenance, and document metadata for your structured extraction, but the canonical indexed text must come from textContentRef. Call embedding_Embed with the same contentRef as inputRef or contentRef and without setting dimensions, then immediately call indexer_IndexDocument with the same file's embeddingRef and textContentRef before moving to the next file. Never pass a filesystem path, filename, document title, or file text as inputRef, contentRef, textContentRef, embeddingRef, or queryVectorRef; every *Ref value must be copied exactly from a previous tool result in this same session. Use at most one service function call per assistant turn, and every service function argument payload must be one valid JSON object with quoted keys and schema-compatible values. In sourceMetadata include filename, path, relative_path when known, document_type, source_hash when known, embedding_provider, embedding_model, embedding_dimensions, embedding_content_hash from the embedding result, and source_content_ref. Do not rename files, restructure the directory, write sidecars, or call services directly outside tools. Do not send a final answer until there are %d successful indexer_IndexDocument results, one for each Markdown file. Reply briefly with the filenames that are ready for questions.`, directory, documentCount)
}

func indexedMarkdownQuestionPrompt() string {
	return `Search the indexed IT company documents and answer from the indexed context only.

Return a concise structured answer covering:
- Which invoice is for Northwind Retail GmbH, what work was billed, and what total is due?
- Which receipt came from ByteWorks Supply GmbH, what equipment was purchased, and what was the total paid?
- Which QuarkOps catalog item has SKU QOP-OBS-START, and what are its monthly price and SLA?
- Which support contract covers Acme Manufacturing AG, what plan is it on, and what is the critical incident response target?

Do not re-read the source files. First embed this question with embedding_Embed. Then call indexer_GetContext with queryVectorRef, limit 10, and depth 1. Use the returned reasoningContext only, and include source filenames when available.`
}
