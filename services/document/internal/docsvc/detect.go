package docsvc

import (
	"crypto/sha256"
	"encoding/hex"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func detectSource(source sourceDocument) detection {
	metadata := cloneMap(source.Metadata)
	if metadata == nil {
		metadata = make(map[string]string)
	}
	hash := sourceHash(source.Content)
	if hash != "" {
		metadata["source_hash"] = hash
	}
	if source.Filename != "" {
		metadata["filename"] = source.Filename
	}
	if source.SourceURI != "" {
		metadata["source_uri"] = source.SourceURI
	}

	extension := strings.ToLower(filepath.Ext(source.Filename))
	mimeType := strings.TrimSpace(source.MIMEType)
	confidence := float32(0.95)
	if mimeType == "" && extension != "" {
		mimeType = mime.TypeByExtension(extension)
		confidence = 0.85
	}
	if mimeType == "" {
		mimeType = detectMIME(source.Content)
		confidence = 0.75
	}
	family := familyFor(mimeType, extension, source.Content)
	return detection{
		MIMEType:   mimeType,
		Extension:  strings.TrimPrefix(extension, "."),
		Family:     family,
		Confidence: confidence,
		Metadata:   metadata,
	}
}

func detectMIME(content []byte) string {
	if len(content) >= 5 && string(content[:5]) == "%PDF-" {
		return "application/pdf"
	}
	if utf8.Valid(content) {
		return "text/plain; charset=utf-8"
	}
	if len(content) > 0 {
		return http.DetectContentType(content)
	}
	return ""
}

func familyFor(mimeType, extension string, content []byte) string {
	base := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch {
	case base == "application/pdf" || extension == ".pdf" || hasPDFMagic(content):
		return "pdf"
	case base == "text/markdown" || extension == ".md" || extension == ".markdown":
		return "markdown"
	case strings.HasPrefix(base, "text/"):
		return "text"
	case strings.HasPrefix(base, "image/"):
		return "image"
	default:
		return "binary"
	}
}

func hasPDFMagic(content []byte) bool {
	return len(content) >= 5 && string(content[:5]) == "%PDF-"
}

func sourceHash(content []byte) string {
	if len(content) == 0 {
		return ""
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func documentID(hash string) string {
	if len(hash) >= 24 {
		return "doc-" + hash[:24]
	}
	if hash != "" {
		return "doc-" + hash
	}
	return ""
}
