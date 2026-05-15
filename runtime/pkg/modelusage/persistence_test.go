package modelusage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/runtime/pkg/modelservice"
)

func TestPersistWritesRedactedUsageToSpaceStorage(t *testing.T) {
	store := &fakeStore{}
	usage := modelservice.Usage{
		SessionID:    "session/1",
		Provider:     "openrouter",
		Model:        "openai/gpt-test",
		InputTokens:  10,
		OutputTokens: 5,
		FinishReason: "stop",
	}

	at := time.Date(2026, 5, 15, 10, 11, 12, 13, time.UTC)
	if err := Persist(context.Background(), store, "space-1", usage, at); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if store.space != "space-1" || store.namespace != Namespace {
		t.Fatalf("store target = %s/%s", store.space, store.namespace)
	}
	if strings.Contains(store.key, "/") {
		t.Fatalf("usage key must be a single supervisor KB path segment, got: %s", store.key)
	}
	if !strings.Contains(store.key, "session_1__runless__20260515T101112.000000013Z__openrouter__openai_gpt-test") {
		t.Fatalf("unexpected key: %s", store.key)
	}
	var got modelservice.Usage
	if err := json.Unmarshal(store.value, &got); err != nil {
		t.Fatalf("decode usage: %v", err)
	}
	if got.Provider != "openrouter" || got.Model != "openai/gpt-test" || got.InputTokens != 10 {
		t.Fatalf("usage = %+v", got)
	}
	if strings.Contains(string(store.value), "prompt") || strings.Contains(string(store.value), "arguments") {
		t.Fatalf("usage persistence leaked non-accounting payload: %s", store.value)
	}
}

type fakeStore struct {
	space     string
	namespace string
	key       string
	value     []byte
}

func (s *fakeStore) KBSet(_ context.Context, space, namespace, key string, value []byte) error {
	s.space = space
	s.namespace = namespace
	s.key = key
	s.value = append([]byte(nil), value...)
	return nil
}
