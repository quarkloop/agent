//go:build e2e

package e2e

import (
	"fmt"
	"strings"
)

func indexPDFDocumentsPrompt(documents []indexedPDFDocument) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Please index this uploaded PDF dataset so I can ask questions about it later. Treat these %d files as one batch.\n\n", len(documents))
	b.WriteString("Documents:\n")
	for _, document := range documents {
		fmt.Fprintf(&b, "- %s (%s)\n", document.Path, document.Name)
	}
	b.WriteString("\nThese paths are the uploaded files to process. Read each PDF, understand what kind of document it is, extract the important facts, people, organizations, topics, and source evidence, and add each file to the knowledge index. Work through the listed files efficiently as one batch where possible. Keep the original files unchanged; do not rename them, reorganize the directory, or create sidecar files. When all files are ready, reply briefly that the dataset is indexed and ready for questions.")
	return b.String()
}

func indexedPDFQuestionPrompt(question string) string {
	return fmt.Sprintf(`Search the indexed PDFs and answer this question from the indexed context:

%s

Use the knowledge you already indexed. Answer in a compact bullet list and include the source filename when it helps verify the answer.`, question)
}
