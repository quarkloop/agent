// Package store provides a unified JSONL-backed key-value store used by all
// subsystems in quark (spaces, snapshots, etc.).
//
// # Design
//
// Each collection is a single append-only JSONL file on disk:
//
//	<dir>/<collection>.jsonl
//
// Every line is a self-contained JSON object with at minimum an "id" field.
// On open the file is loaded into memory (a map[string]json.RawMessage keyed by
// the record's "id" field). All reads are served from memory; all writes are
// flushed to disk immediately by rewriting the file atomically (write to a
// .tmp sibling, then os.Rename). This keeps the code small, the format human-
// readable, and the dependency list empty.
//
// # Concurrency
//
// A single sync.RWMutex guards each Collection. Multiple goroutines may read
// concurrently; writes are serialised.
//
// # File format
//
// Each record occupies exactly one line. The file is rewritten on every
// mutation (Set / Delete). For the expected record counts in quark (tens to
// low-hundreds) this is fast and safe. If a collection needs streaming-append
// semantics in the future, the interface is unchanged — only the flush
// implementation would change.
package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// idField is the JSON key used to uniquely identify records within a collection.
const idField = "id"

// Collection is a named, persistent set of JSON records stored in a JSONL file.
// The zero value is not usable; obtain one via Open.
type Collection struct {
	path    string // absolute path to the .jsonl file
	mu      sync.RWMutex
	records map[string]json.RawMessage // id → raw JSON
	order   []string                   // insertion order (for deterministic List)
}

// Open opens (or creates) the JSONL collection at <dir>/<name>.jsonl.
// dir is created if it does not exist.
func Open(dir, name string) (*Collection, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("store: mkdir %s: %w", dir, err)
	}
	c := &Collection{
		path:    filepath.Join(dir, name+".jsonl"),
		records: map[string]json.RawMessage{},
	}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

// Set upserts a record. The value must be a JSON-marshalable type and must
// contain an "id" field (string) at the top level — Set returns an error if
// the field is missing or empty after marshalling.
func (c *Collection) Set(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("store: marshal: %w", err)
	}
	id, err := extractID(raw)
	if err != nil {
		return fmt.Errorf("store Set: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.records[id]; !exists {
		c.order = append(c.order, id)
	}
	c.records[id] = raw
	return c.flush()
}

// Get retrieves the raw JSON bytes for the record with the given id.
// Returns (nil, ErrNotFound) when absent.
func (c *Collection) Get(id string) (json.RawMessage, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	raw, ok := c.records[id]
	if !ok {
		return nil, NewNotFoundError(id)
	}
	return raw, nil
}

// GetInto retrieves the record with the given id and unmarshals it into dst.
func (c *Collection) GetInto(id string, dst any) error {
	raw, err := c.Get(id)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

// Delete removes the record with the given id.
// Returns ErrNotFound when absent.
func (c *Collection) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.records[id]; !ok {
		return NewNotFoundError(id)
	}
	delete(c.records, id)
	// Remove from order slice
	for i, v := range c.order {
		if v == id {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	return c.flush()
}

// List returns raw JSON for every record in insertion order.
func (c *Collection) List() []json.RawMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]json.RawMessage, 0, len(c.order))
	for _, id := range c.order {
		if raw, ok := c.records[id]; ok {
			out = append(out, raw)
		}
	}
	return out
}

// ListIDs returns the id of every record in insertion order.
func (c *Collection) ListIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ids := make([]string, len(c.order))
	copy(ids, c.order)
	return ids
}

// Count returns the number of records in the collection.
func (c *Collection) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.records)
}

// Close is a no-op; provided so Collection satisfies io.Closer-style patterns
// and allows future cleanup logic without changing call sites.
func (c *Collection) Close() error { return nil }

// ── internal ──────────────────────────────────────────────────────────────────

// load reads the JSONL file from disk into memory. Missing file is not an error.
func (c *Collection) load() error {
	f, err := os.Open(c.path)
	if os.IsNotExist(err) {
		return nil // empty collection is fine
	}
	if err != nil {
		return fmt.Errorf("store: open %s: %w", c.path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB per line
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		id, err := extractID(line)
		if err != nil {
			continue // skip malformed lines
		}
		if _, exists := c.records[id]; !exists {
			c.order = append(c.order, id)
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		c.records[id] = cp
	}
	return sc.Err()
}

// flush atomically rewrites the JSONL file with current in-memory records.
// Caller must hold c.mu (write lock).
func (c *Collection) flush() error {
	tmp := c.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("store: create tmp %s: %w", tmp, err)
	}
	w := bufio.NewWriter(f)
	for _, id := range c.order {
		raw, ok := c.records[id]
		if !ok {
			continue
		}
		if _, err := w.Write(raw); err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("store: write: %w", err)
		}
		if err := w.WriteByte('\n'); err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("store: write newline: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("store: flush: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("store: close tmp: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("store: rename: %w", err)
	}
	return nil
}

// extractID parses the "id" field from a raw JSON object.
func extractID(raw []byte) (string, error) {
	var probe struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", fmt.Errorf("extractID: %w", err)
	}
	if probe.ID == "" {
		return "", fmt.Errorf("record missing non-empty %q field", idField)
	}
	return probe.ID, nil
}
