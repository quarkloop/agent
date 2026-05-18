//go:build e2e

package e2e

import "fmt"

func indexMarkdownDirectoryPrompt(directory string, documentCount int) string {
	return fmt.Sprintf(`Please index the Markdown documents in this company records directory for later structured Q&A:

%s

Please find every Markdown file in that directory, read each one, understand what kind of business record it is, extract the important facts and source evidence, and add all %d documents to the knowledge index. Keep the directory unchanged; do not rename files, reorganize folders, or create sidecar files. When the batch is ready, reply briefly with the filenames I can ask questions about.`, directory, documentCount)
}

func indexedMarkdownQuestionPrompt() string {
	return `Search the indexed IT company documents and answer from the indexed context only.

Return one compact bullet for each of these questions:
- Which invoice is for Northwind Retail GmbH, what work was billed, and what total is due?
- Which receipt came from ByteWorks Supply GmbH, what equipment was purchased, and what was the total paid?
- Which QuarkOps catalog item has SKU QOP-OBS-START, and what are its monthly price and SLA?
- Which support contract covers Acme Manufacturing AG, what plan is it on, and what is the critical incident response target?

Use the indexed knowledge, keep each bullet short, and include source filenames when available.`
}
