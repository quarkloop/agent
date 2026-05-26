//go:build e2e

package e2e

import (
	"path/filepath"
	"testing"
)

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
