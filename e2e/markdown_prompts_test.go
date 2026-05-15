//go:build e2e

package e2e

import "fmt"

func indexMarkdownDirectoryPrompt(directory string, documentCount int) string {
	return fmt.Sprintf(`Please index the Markdown documents in this company records directory for later structured Q&A:

%s

Treat every .md file under that directory as one user upload batch. Required workflow: first call fs list on the directory with recursive=true and include_hash=true. Then read every discovered .md file with fs read. For each Markdown file, create one compact searchable text chunk that preserves document identifiers, companies, products, dates, prices, totals, response targets, and source filename. Call embedding_Embed without setting dimensions for that chunk, then immediately call indexer_IndexDocument with the same file's embeddingRef before moving to the next file. In sourceMetadata include filename, path, relative_path when known, document_type, source_hash when known, embedding_provider, embedding_model, embedding_dimensions, and embedding_content_hash from the embedding result. Do not rename files, restructure the directory, write sidecars, or call services directly outside tools. Do not send a final answer until there are %d successful indexer_IndexDocument results, one for each Markdown file. Reply briefly with the filenames that are ready for questions.`, directory, documentCount)
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
