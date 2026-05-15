package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func EntityIDFromName(name string) string {
	id := strings.ToLower(strings.TrimSpace(name))
	id = strings.ReplaceAll(id, " ", "-")
	if id == "" {
		return ""
	}
	return id
}

func SourceHashFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}
