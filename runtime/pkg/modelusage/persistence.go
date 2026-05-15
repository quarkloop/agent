package modelusage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/quarkloop/runtime/pkg/modelservice"
)

const Namespace = "model_usage"

type Store interface {
	KBSet(ctx context.Context, space, namespace, key string, value []byte) error
}

// Persist writes one redacted usage record into supervisor-owned space storage.
func Persist(ctx context.Context, store Store, space string, usage modelservice.Usage, at time.Time) error {
	if store == nil || strings.TrimSpace(space) == "" {
		return nil
	}
	data, err := json.Marshal(usage)
	if err != nil {
		return fmt.Errorf("marshal model usage: %w", err)
	}
	if err := store.KBSet(ctx, space, Namespace, Key(usage, at), data); err != nil {
		return fmt.Errorf("persist model usage: %w", err)
	}
	return nil
}

func Key(usage modelservice.Usage, at time.Time) string {
	sessionID := strings.TrimSpace(usage.SessionID)
	if sessionID == "" {
		sessionID = "sessionless"
	}
	runID := strings.TrimSpace(usage.RunID)
	if runID == "" {
		runID = "runless"
	}
	provider := strings.TrimSpace(usage.Provider)
	if provider == "" {
		provider = "providerless"
	}
	model := strings.TrimSpace(usage.Model)
	if model == "" {
		model = "modelless"
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return sanitize(sessionID) + "/" +
		sanitize(runID) + "/" +
		at.UTC().Format("20060102T150405.000000000Z") + "/" +
		sanitize(provider) + "/" +
		sanitize(model)
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
