package ingestionsvc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func newRunID(now time.Time) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		sum := sha256.Sum256([]byte(now.Format(time.RFC3339Nano)))
		return "ing-" + hex.EncodeToString(sum[:8])
	}
	return "ing-" + hex.EncodeToString(b[:])
}

func sourceID(seed string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", seed, index)))
	return "src-" + hex.EncodeToString(sum[:8])
}

func uniqueSourceID(base string, seen map[string]int) string {
	count := seen[base]
	seen[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count+1)
}

func lowerPhase(ph phase) string {
	return strings.ReplaceAll(string(ph), "_", "-")
}
