package runstatesvc

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
		return "run-" + hex.EncodeToString(sum[:8])
	}
	return "run-" + hex.EncodeToString(b[:])
}

func itemID(seed string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", seed, index)))
	return "item-" + hex.EncodeToString(sum[:8])
}

func uniqueID(base string, seen map[string]int) string {
	count := seen[base]
	seen[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count+1)
}

func leaseKey(runID, itemID string) string {
	if strings.TrimSpace(itemID) == "" {
		return "run." + strings.TrimSpace(runID)
	}
	return "run." + strings.TrimSpace(runID) + ".item." + strings.TrimSpace(itemID)
}
