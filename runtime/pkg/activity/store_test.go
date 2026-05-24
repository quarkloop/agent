package activity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStoreBoundsAndCopiesRecords(t *testing.T) {
	store := NewStore(2)
	store.Add("s1", "first", map[string]string{"value": "one"})
	store.Add("s1", "second", map[string]string{"value": "two"})
	store.Add("s1", "third", map[string]string{"value": "three"})

	records := store.List(0)
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].Type != "second" || records[1].Type != "third" {
		t.Fatalf("unexpected records: %+v", records)
	}

	records[0].Data = json.RawMessage(`{"mutated":true}`)
	again := store.List(1)
	if string(again[0].Data) == string(records[0].Data) {
		t.Fatal("store exposed mutable record data")
	}
}

func TestStorePublishesToSubscribers(t *testing.T) {
	store := NewStore(10)
	ch := store.Subscribe()
	defer store.Unsubscribe(ch)

	record := store.Add("s1", "message.user", map[string]string{"role": "user"})
	got := <-ch
	if got.ID != record.ID || got.Type != "message.user" {
		t.Fatalf("subscriber got %+v, want %+v", got, record)
	}
}

func TestStoreRedactsSecretsAtActivityBoundary(t *testing.T) {
	secret := "sk-or-v1-runtime-secret"
	t.Setenv("OPENROUTER_API_KEY", secret)
	store := NewStore(10)

	record := store.Add("s1", "tool_start", map[string]any{
		"name":      "gateway_Embed",
		"arguments": `{"authorization":"Bearer ` + secret + `"}`,
	})

	data := string(record.Data)
	if strings.Contains(data, secret) {
		t.Fatalf("activity data leaked secret: %s", data)
	}
	if !strings.Contains(data, `Bearer [redacted]`) {
		t.Fatalf("activity data was not redacted: %s", data)
	}
}
