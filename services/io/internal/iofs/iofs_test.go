package iofs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListEntriesSupportsReadOnlyDirectoryHashAndTimestamps(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	file := filepath.Join(nested, "source.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.Chmod(nested, 0o555); err != nil {
		t.Fatalf("chmod nested read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(nested, 0o755) })

	result, err := List(dir, true, true)
	if err != nil {
		t.Fatalf("list read-only directory: %v", err)
	}
	var found FileEntry
	for _, entry := range result.Files {
		if entry.RelativePath == "nested/source.txt" {
			found = entry
			break
		}
	}
	if found.RelativePath == "" {
		t.Fatalf("source file missing from recursive entries: %+v", result.Files)
	}
	if found.SHA256 == "" || found.Modified == "" || found.Path == "" {
		t.Fatalf("entry missing identity fields: %+v", found)
	}
}

func TestReadWorksInReadOnlyDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod read-only dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	result, err := Read(path, 0, 0)
	if err != nil {
		t.Fatalf("read read-only source: %v", err)
	}
	if result.Content != "hello" {
		t.Fatalf("content = %q, want hello", result.Content)
	}
}

func TestMutatingCommandsRequireExplicitApproval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := Write(filepath.Join(dir, "new.txt"), "new", false); err != ErrMutationNotApproved {
		t.Fatalf("write err = %v", err)
	}
	if _, err := Append(path, "!", false); err != ErrMutationNotApproved {
		t.Fatalf("append err = %v", err)
	}
	if _, err := Replace(path, "hello", "bye", false); err != ErrMutationNotApproved {
		t.Fatalf("replace err = %v", err)
	}
	if err := Remove(path, false); err != ErrMutationNotApproved {
		t.Fatalf("remove err = %v", err)
	}
}

func TestMutatingCommandsRequireSafePath(t *testing.T) {
	if _, err := Write("", "new", true); err != ErrInvalidPath {
		t.Fatalf("write err = %v", err)
	}
	if _, err := Append("   ", "new", true); err != ErrInvalidPath {
		t.Fatalf("append err = %v", err)
	}
	if _, err := Replace(".", "old", "new", true); err == nil {
		t.Fatal("replace on current directory unexpectedly succeeded")
	}
	if err := Remove(".", true); err == nil {
		t.Fatal("remove current directory unexpectedly succeeded")
	}
}

func TestMutatingCommandsRunWithExplicitApproval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")

	if _, err := Write(path, "hello", true); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Append(path, "!", true); err != nil {
		t.Fatalf("append: %v", err)
	}
	assertFileContent(t, path, "hello!")
	if _, err := Replace(path, "hello", "bye", true); err != nil {
		t.Fatalf("replace: %v", err)
	}
	assertFileContent(t, path, "bye!")
	if err := Remove(path, true); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestReplaceRejectsEmptyFindWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := Replace(path, "", "x", true); err == nil {
		t.Fatal("replace with empty find unexpectedly succeeded")
	}
	assertFileContent(t, path, "hello")
}

func TestReplaceSkipsWriteWhenNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	count, err := Replace(path, "missing", "x", true)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if count != 0 {
		t.Fatalf("replacements = %d, want 0", count)
	}
	assertFileContent(t, path, "hello")
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s content = %q, want %q", path, string(data), want)
	}
}
