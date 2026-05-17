package docsvc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type pdfExtractor interface {
	ExtractText(ctx context.Context, source sourceDocument) (string, error)
}

type commandPDFExtractor struct {
	binary string
}

func newCommandPDFExtractor(binary string) commandPDFExtractor {
	return commandPDFExtractor{binary: strings.TrimSpace(binary)}
}

func (e commandPDFExtractor) ExtractText(ctx context.Context, source sourceDocument) (string, error) {
	binary := e.binary
	if binary == "" {
		found, err := exec.LookPath("pdftotext")
		if err != nil {
			return "", errPDFBackendMissing
		}
		binary = found
	}

	path := source.Path
	cleanup := func() {}
	if path == "" {
		tempDir, err := os.MkdirTemp("", "quark-document-pdf-*")
		if err != nil {
			return "", fmt.Errorf("create pdf temp dir: %w", err)
		}
		cleanup = func() { _ = os.RemoveAll(tempDir) }
		path = filepath.Join(tempDir, "source.pdf")
		if err := os.WriteFile(path, source.Content, 0o600); err != nil {
			cleanup()
			return "", fmt.Errorf("write pdf temp file: %w", err)
		}
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, binary, "-layout", path, "-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("%w: %v: %s", errPDFParseFailed, err, msg)
		}
		return "", fmt.Errorf("%w: %v", errPDFParseFailed, err)
	}
	return strings.TrimRight(string(out), "\x00"), nil
}
