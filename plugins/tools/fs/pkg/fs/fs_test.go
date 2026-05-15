package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/quarkloop/pkg/toolkit"
)

func TestSchemaIncludesPDFExtraction(t *testing.T) {
	schema := (&Tool{}).Schema()
	props, ok := schema.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %#v", schema.Parameters)
	}
	command, ok := props["command"].(map[string]any)
	if !ok {
		t.Fatalf("command schema missing: %#v", props["command"])
	}
	if approved, ok := props["approved"].(map[string]any); !ok || approved["type"] != "boolean" {
		t.Fatalf("approved schema missing: %#v", props["approved"])
	}
	enum, ok := command["enum"].([]string)
	if !ok {
		t.Fatalf("command enum missing: %#v", command["enum"])
	}
	for _, value := range enum {
		if value == "extract_pdf" {
			return
		}
	}
	t.Fatalf("command enum missing extract_pdf: %#v", enum)
}

func TestIntFlagAcceptsSchemaAlias(t *testing.T) {
	got, err := intFlag(map[string]any{"max_chars": float64(1200)}, "max-chars", defaultPDFMaxChars)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1200 {
		t.Fatalf("max chars = %d, want 1200", got)
	}
}

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

	entries, err := listEntries(dir, true, true)
	if err != nil {
		t.Fatalf("list read-only directory: %v", err)
	}
	var found fileEntry
	for _, entry := range entries {
		if entry.RelativePath == "nested/source.txt" {
			found = entry
			break
		}
	}
	if found.RelativePath == "" {
		t.Fatalf("source file missing from recursive entries: %+v", entries)
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

	out, err := handleRead(toolkitMutationInput(path, nil))
	if err != nil {
		t.Fatalf("read read-only source: %v", err)
	}
	if out.Error != "" || out.Data["content"] != "hello" {
		t.Fatalf("read output = %+v", out)
	}
}

func TestStatIncludesSHA256ForRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out, err := handleStat(toolkitInput(path, true))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if out.Error != "" {
		t.Fatalf("stat returned error: %s", out.Error)
	}
	if out.Data["sha256"] == "" {
		t.Fatalf("stat missing sha256: %+v", out.Data)
	}
}

func TestMutatingCommandsRequireExplicitApproval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cases := []struct {
		name   string
		run    func() toolkit.Output
		assert func(t *testing.T)
	}{
		{
			name: "write",
			run: func() toolkit.Output {
				out, _ := writeCommand(t).Handler(nil, toolkitMutationInput(filepath.Join(dir, "new.txt"), map[string]string{"content": "new"}))
				return out
			},
			assert: func(t *testing.T) {
				if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
					t.Fatalf("write created file without approval: %v", err)
				}
			},
		},
		{
			name: "append",
			run: func() toolkit.Output {
				out, _ := appendCommand(t).Handler(nil, toolkitMutationInput(path, map[string]string{"content": "!"}))
				return out
			},
			assert: func(t *testing.T) { assertFileContent(t, path, "hello") },
		},
		{
			name: "replace",
			run: func() toolkit.Output {
				out, _ := replaceCommand(t).Handler(nil, toolkitMutationInput(path, map[string]string{"find": "hello", "replace-with": "bye"}))
				return out
			},
			assert: func(t *testing.T) { assertFileContent(t, path, "hello") },
		},
		{
			name: "rm",
			run: func() toolkit.Output {
				out, _ := rmCommand(t).Handler(nil, toolkitMutationInput(path, nil))
				return out
			},
			assert: func(t *testing.T) {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("rm removed file without approval: %v", err)
				}
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.run()
			if out.Error != mutationApprovalRequired {
				t.Fatalf("error = %q, want %q", out.Error, mutationApprovalRequired)
			}
			tt.assert(t)
		})
	}
}

func TestMutatingCommandsRunWithExplicitApproval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")

	writeOut, _ := writeCommand(t).Handler(nil, toolkitMutationInput(path, map[string]string{
		"content":  "hello",
		"approved": "true",
	}))
	if writeOut.Error != "" {
		t.Fatalf("write with approval returned error: %s", writeOut.Error)
	}
	assertFileContent(t, path, "hello")

	appendOut, _ := appendCommand(t).Handler(nil, toolkitMutationInput(path, map[string]string{
		"content":  "!",
		"approved": "true",
	}))
	if appendOut.Error != "" {
		t.Fatalf("append with approval returned error: %s", appendOut.Error)
	}
	assertFileContent(t, path, "hello!")

	replaceOut, _ := replaceCommand(t).Handler(nil, toolkitMutationInput(path, map[string]string{
		"find":         "hello",
		"replace-with": "bye",
		"approved":     "true",
	}))
	if replaceOut.Error != "" {
		t.Fatalf("replace with approval returned error: %s", replaceOut.Error)
	}
	assertFileContent(t, path, "bye!")

	rmOut, _ := rmCommand(t).Handler(nil, toolkitMutationInput(path, map[string]string{"approved": "true"}))
	if rmOut.Error != "" {
		t.Fatalf("rm with approval returned error: %s", rmOut.Error)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rm with approval left file behind: %v", err)
	}
}

func toolkitInput(path string, includeHash bool) toolkit.Input {
	return toolkit.Input{
		Args:  map[string]string{"path": path},
		Flags: map[string]any{"include-hash": includeHash},
	}
}

func toolkitMutationInput(path string, values map[string]string) toolkit.Input {
	input := toolkit.Input{
		Args: map[string]string{
			"path": path,
		},
		Flags: map[string]any{
			"approved": false,
		},
	}
	for key, value := range values {
		switch key {
		case "approved":
			input.Flags["approved"] = value == "true"
		default:
			input.Args[key] = value
		}
	}
	return input
}

func writeCommand(t *testing.T) toolkit.Command {
	t.Helper()
	return commandByName(t, "write")
}

func appendCommand(t *testing.T) toolkit.Command {
	t.Helper()
	return commandByName(t, "append")
}

func replaceCommand(t *testing.T) toolkit.Command {
	t.Helper()
	return commandByName(t, "replace")
}

func rmCommand(t *testing.T) toolkit.Command {
	t.Helper()
	return commandByName(t, "rm")
}

func commandByName(t *testing.T, name string) toolkit.Command {
	t.Helper()
	for _, command := range (&Tool{}).Commands() {
		if command.Name == name {
			return command
		}
	}
	t.Fatalf("command %s missing", name)
	return toolkit.Command{}
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
