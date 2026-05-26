//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

type markdownDocumentFixture struct {
	Name   string
	Source string
	Query  string
	Want   []string
}

type indexedMarkdownDocument struct {
	Name     string
	Path     string
	Filename string
	Query    string
	Want     []string
}

func copyMarkdownDocuments(t *testing.T, dir string, fixtures []markdownDocumentFixture) []indexedMarkdownDocument {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir markdown fixture dir: %v", err)
	}
	out := make([]indexedMarkdownDocument, 0, len(fixtures))
	for _, fixture := range fixtures {
		filename := filepath.Base(fixture.Source)
		dst := filepath.Join(dir, filename)
		copyTestFile(t, fixture.Source, dst)
		out = append(out, indexedMarkdownDocument{
			Name:     fixture.Name,
			Path:     dst,
			Filename: filename,
			Query:    fixture.Query,
			Want:     append([]string(nil), fixture.Want...),
		})
	}
	return out
}
