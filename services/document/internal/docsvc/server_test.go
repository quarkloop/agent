package docsvc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/boundary"
	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
)

type fakePDFExtractor struct {
	text string
	err  error
}

func (f fakePDFExtractor) ExtractText(context.Context, sourceDocument) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}

func testServer(pdf pdfExtractor) *Server {
	return &Server{parser: newParser(pdf)}
}

func TestDetectTypeFromFilenameWithoutContent(t *testing.T) {
	srv := testServer(fakePDFExtractor{})
	resp, err := srv.DetectType(context.Background(), &documentv1.DetectTypeRequest{
		Input: &documentv1.DocumentInput{Filename: "report.pdf"},
	})
	if err != nil {
		t.Fatalf("DetectType returned error: %v", err)
	}
	if resp.GetDocumentFamily() != "pdf" {
		t.Fatalf("family = %q, want pdf", resp.GetDocumentFamily())
	}
	if resp.GetExtension() != "pdf" {
		t.Fatalf("extension = %q, want pdf", resp.GetExtension())
	}
}

func TestMarkdownExtractionTablesLayoutAndOCRTextLayer(t *testing.T) {
	srv := testServer(fakePDFExtractor{})
	content := []byte("# Roadmap\n\n| Name | Status |\n| --- | --- |\n| Core | Done |\n| Document | Active |\n")
	input := &documentv1.DocumentInput{Filename: "roadmap.md", Content: content}

	parsed, err := srv.ParseBytes(context.Background(), &documentv1.ParseBytesRequest{Input: input})
	if err != nil {
		t.Fatalf("ParseBytes returned error: %v", err)
	}
	if parsed.GetDocumentFamily() != "markdown" {
		t.Fatalf("family = %q, want markdown", parsed.GetDocumentFamily())
	}
	if parsed.GetDocumentId() == "" || parsed.GetSourceHash() == "" {
		t.Fatalf("expected stable document id and source hash: %#v", parsed)
	}

	text, err := srv.ExtractText(context.Background(), &documentv1.ExtractTextRequest{Input: input, MaxChars: 32})
	if err != nil {
		t.Fatalf("ExtractText returned error: %v", err)
	}
	if !strings.Contains(text.GetText(), "Roadmap") {
		t.Fatalf("text = %q, want Roadmap", text.GetText())
	}
	if len(text.GetText()) > 32 {
		t.Fatalf("text length = %d, want max 32", len(text.GetText()))
	}
	if len(text.GetPages()) != 1 {
		t.Fatalf("pages = %d, want 1", len(text.GetPages()))
	}
	if text.GetPages()[0].GetSource().GetPageNumber() != 1 || text.GetSource().GetModality() != "text" {
		t.Fatalf("text source references = %#v %#v", text.GetSource(), text.GetPages()[0].GetSource())
	}

	tables, err := srv.ExtractTables(context.Background(), &documentv1.ExtractTablesRequest{Input: input})
	if err != nil {
		t.Fatalf("ExtractTables returned error: %v", err)
	}
	if len(tables.GetTables()) != 1 {
		t.Fatalf("tables = %d, want 1", len(tables.GetTables()))
	}
	if got := tables.GetTables()[0].GetHeaders(); len(got) != 2 || got[0] != "Name" || got[1] != "Status" {
		t.Fatalf("headers = %#v, want markdown headers", got)
	}

	layout, err := srv.ExtractLayout(context.Background(), &documentv1.ExtractLayoutRequest{Input: input})
	if err != nil {
		t.Fatalf("ExtractLayout returned error: %v", err)
	}
	if len(layout.GetPages()) != 1 || len(layout.GetPages()[0].GetBlocks()) == 0 {
		t.Fatalf("expected layout blocks, got %#v", layout.GetPages())
	}

	ocr, err := srv.RunOCR(context.Background(), &documentv1.RunOCRRequest{Input: input})
	if err != nil {
		t.Fatalf("RunOCR returned error: %v", err)
	}
	if ocr.GetEngine() != "text-layer" || ocr.GetConfidence() != 1 {
		t.Fatalf("ocr = %#v, want text-layer confidence 1", ocr)
	}
}

func TestPlainTextPagesAndDelimitedTables(t *testing.T) {
	srv := testServer(fakePDFExtractor{})
	content := []byte("Name\tScore\nAda\t10\nGrace\t9\n")
	resp, err := srv.GetPages(context.Background(), &documentv1.GetPagesRequest{
		Input: &documentv1.DocumentInput{Filename: "scores.txt", Content: content},
	})
	if err != nil {
		t.Fatalf("GetPages returned error: %v", err)
	}
	if len(resp.GetPages()) != 1 {
		t.Fatalf("pages = %d, want 1", len(resp.GetPages()))
	}
	if len(resp.GetPages()[0].GetTables()) != 1 {
		t.Fatalf("tables = %d, want 1", len(resp.GetPages()[0].GetTables()))
	}
}

func TestPDFExtractionWithFakeTextLayer(t *testing.T) {
	srv := testServer(fakePDFExtractor{text: "Page one\fPage two"})
	resp, err := srv.ExtractText(context.Background(), &documentv1.ExtractTextRequest{
		Input: &documentv1.DocumentInput{Filename: "paper.pdf", Content: []byte("%PDF-1.7 fake")},
	})
	if err != nil {
		t.Fatalf("ExtractText returned error: %v", err)
	}
	if got := len(resp.GetPages()); got != 2 {
		t.Fatalf("pages = %d, want 2", got)
	}
	if !strings.Contains(resp.GetText(), "Page two") {
		t.Fatalf("text = %q, want second page text", resp.GetText())
	}
}

func TestRealPDFFixtureWhenBackendExists(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}
	fixture := filepath.Join("..", "..", "..", "..", "e2e", "testdata", "documents", "attention_is_all_you_need_paper.pdf")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("pdf fixture unavailable: %v", err)
	}
	srv := NewServer(Config{})
	resp, err := srv.ExtractText(context.Background(), &documentv1.ExtractTextRequest{
		Input: &documentv1.DocumentInput{SourceUri: fixture, Filename: filepath.Base(fixture)},
	})
	if err != nil {
		t.Fatalf("ExtractText returned error: %v", err)
	}
	text := strings.ToLower(resp.GetText())
	if !strings.Contains(text, "attention") || !strings.Contains(text, "transformer") {
		t.Fatalf("fixture text did not include expected paper terms")
	}
	if len(resp.GetPages()) == 0 {
		t.Fatalf("expected page records for PDF fixture")
	}
}

func TestMalformedPDFReturnsInvalidArgument(t *testing.T) {
	srv := testServer(fakePDFExtractor{err: errPDFParseFailed})
	_, err := srv.ParseBytes(context.Background(), &documentv1.ParseBytesRequest{
		Input: &documentv1.DocumentInput{Filename: "bad.pdf", Content: []byte("%PDF-1.7 broken")},
	})
	if !boundary.IsCategory(err, boundary.InvalidArgument) {
		t.Fatalf("error = %v, want InvalidArgument", err)
	}
}

func TestEmptyUnsupportedAndImageOCRFailures(t *testing.T) {
	srv := testServer(fakePDFExtractor{})
	_, err := srv.ParseBytes(context.Background(), &documentv1.ParseBytesRequest{
		Input: &documentv1.DocumentInput{Filename: "empty.txt", Content: nil},
	})
	if !boundary.IsCategory(err, boundary.InvalidArgument) {
		t.Fatalf("empty error = %v, want InvalidArgument", err)
	}

	_, err = srv.ParseBytes(context.Background(), &documentv1.ParseBytesRequest{
		Input: &documentv1.DocumentInput{Filename: "blob.bin", Content: []byte{0x00, 0xff, 0x10}},
	})
	if !boundary.IsCategory(err, boundary.InvalidArgument) {
		t.Fatalf("unsupported error = %v, want InvalidArgument", err)
	}

	imageInput := &documentv1.DocumentInput{Filename: "scan.png", MimeType: "image/png", Content: []byte{0x89, 'P', 'N', 'G'}}
	images, err := srv.ExtractImages(context.Background(), &documentv1.ExtractImagesRequest{Input: imageInput})
	if err != nil {
		t.Fatalf("ExtractImages returned error: %v", err)
	}
	if len(images.GetImages()) != 1 || images.GetImages()[0].GetImageRef() == "" {
		t.Fatalf("expected image reference, got %#v", images.GetImages())
	}
	if len(images.GetImages()[0].GetContent()) == 0 || images.GetImages()[0].GetSource().GetModality() != "image" {
		t.Fatalf("expected image bytes and typed source reference, got %#v", images.GetImages()[0])
	}
	_, err = srv.RunOCR(context.Background(), &documentv1.RunOCRRequest{Input: imageInput})
	if !boundary.IsCategory(err, boundary.Conflict) {
		t.Fatalf("image OCR error = %v, want FailedPrecondition", err)
	}
}
