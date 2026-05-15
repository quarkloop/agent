package fs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/toolkit"
)

const defaultPDFMaxChars = 30000
const mutationApprovalRequired = "workspace mutation requires explicit user approval"

// Tool implements filesystem operations.
type Tool struct {
	manifest *plugin.Manifest
}

func (t *Tool) SetManifest(m *plugin.Manifest) {
	t.manifest = m
}

// Name returns the tool name.
func (t *Tool) Name() string {
	if t.manifest == nil {
		return "fs"
	}
	return t.manifest.Name
}

// Version returns the tool version.
func (t *Tool) Version() string {
	if t.manifest == nil {
		return "1.0.0"
	}
	return t.manifest.Version
}

// Description returns the tool description.
func (t *Tool) Description() string {
	if t.manifest == nil {
		return "Filesystem operations"
	}
	return t.manifest.Description
}

// Schema returns the tool schema for LLM function calling.
func (t *Tool) Schema() plugin.ToolSchema {
	if t.manifest != nil && t.manifest.Tool != nil {
		return t.manifest.Tool.Schema
	}
	return plugin.ToolSchema{
		Name:        "fs",
		Description: "Read, write, append, replace, list, stat, remove files and directories, and extract text from PDFs",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type": "string",
					"enum": []string{"read", "write", "append", "replace", "list", "stat", "rm", "extract_pdf"},
				},
				"path": map[string]any{
					"type": "string",
				},
				"content": map[string]any{
					"type": "string",
				},
				"find": map[string]any{
					"type": "string",
				},
				"replace_with": map[string]any{
					"type": "string",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"description": "1-based start line for partial read",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"description": "1-based inclusive end line for partial read",
				},
				"max_chars": map[string]any{
					"type":        "integer",
					"description": "Maximum characters to return for PDF extraction; 0 means no limit",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "For list, walk the directory recursively",
				},
				"include_hash": map[string]any{
					"type":        "boolean",
					"description": "For list/stat, include sha256 for regular files",
				},
				"approved": map[string]any{
					"type":        "boolean",
					"description": "Required true for write, append, replace, and rm after explicit user approval",
				},
			},
			"required": []string{"command", "path"},
		},
	}
}

// Commands returns the available filesystem commands.
func (t *Tool) Commands() []toolkit.Command {
	return []toolkit.Command{
		{
			Name:        "read",
			Description: "Read a text file, optionally with line range",
			Args: []toolkit.Arg{
				{Name: "path", Description: "File path", Required: true},
			},
			Flags: []toolkit.Flag{
				{Name: "start-line", Type: "int", Description: "1-based start line (optional)", Default: 0},
				{Name: "end-line", Type: "int", Description: "1-based inclusive end line (optional)", Default: 0},
			},
			Handler: func(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
				return handleRead(input)
			},
		},
		{
			Name:        "extract_pdf",
			Description: "Extract text from a PDF file using pdftotext",
			Args: []toolkit.Arg{
				{Name: "path", Description: "PDF file path", Required: true},
			},
			Flags: []toolkit.Flag{
				{Name: "max-chars", Type: "int", Description: "Maximum characters to return (0 = unlimited)", Default: defaultPDFMaxChars},
			},
			Handler: func(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
				return handleExtractPDF(ctx, input)
			},
		},
		{
			Name:        "write",
			Description: "Write content to a file (overwrite)",
			Args: []toolkit.Arg{
				{Name: "path", Description: "File path", Required: true},
				{Name: "content", Description: "Content to write", Required: true},
			},
			Flags: []toolkit.Flag{
				{Name: "approved", Type: "bool", Description: "Must be true after explicit user approval", Default: false},
			},
			Handler: func(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
				if out, blocked := requireMutationApproval(input); blocked {
					return out, nil
				}
				err := os.WriteFile(input.Args["path"], []byte(input.Args["content"]), 0644)
				if err != nil {
					return toolkit.Output{Error: err.Error()}, nil
				}
				return toolkit.Output{Data: map[string]any{"written": len(input.Args["content"])}}, nil
			},
		},
		{
			Name:        "append",
			Description: "Append content to a file",
			Args: []toolkit.Arg{
				{Name: "path", Description: "File path", Required: true},
				{Name: "content", Description: "Content to append", Required: true},
			},
			Flags: []toolkit.Flag{
				{Name: "approved", Type: "bool", Description: "Must be true after explicit user approval", Default: false},
			},
			Handler: func(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
				if out, blocked := requireMutationApproval(input); blocked {
					return out, nil
				}
				f, err := os.OpenFile(input.Args["path"], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return toolkit.Output{Error: err.Error()}, nil
				}
				defer f.Close()
				n, err := f.WriteString(input.Args["content"])
				if err != nil {
					return toolkit.Output{Error: err.Error()}, nil
				}
				return toolkit.Output{Data: map[string]any{"appended": n}}, nil
			},
		},
		{
			Name:        "replace",
			Description: "Replace all occurrences of text in a file",
			Args: []toolkit.Arg{
				{Name: "path", Description: "File path", Required: true},
				{Name: "find", Description: "Text to find", Required: true},
				{Name: "replace-with", Description: "Replacement text", Required: true},
			},
			Flags: []toolkit.Flag{
				{Name: "approved", Type: "bool", Description: "Must be true after explicit user approval", Default: false},
			},
			Handler: func(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
				if out, blocked := requireMutationApproval(input); blocked {
					return out, nil
				}
				data, err := os.ReadFile(input.Args["path"])
				if err != nil {
					return toolkit.Output{Error: err.Error()}, nil
				}
				replacements := strings.Count(string(data), input.Args["find"])
				newContent := strings.ReplaceAll(string(data), input.Args["find"], input.Args["replace-with"])
				if err := os.WriteFile(input.Args["path"], []byte(newContent), 0644); err != nil {
					return toolkit.Output{Error: err.Error()}, nil
				}
				return toolkit.Output{Data: map[string]any{"replacements": replacements}}, nil
			},
		},
		{
			Name:        "list",
			Description: "List directory contents",
			Args: []toolkit.Arg{
				{Name: "path", Description: "Directory path", Required: false, Default: "."},
			},
			Flags: []toolkit.Flag{
				{Name: "recursive", Type: "bool", Description: "Walk directory recursively", Default: false},
				{Name: "include-hash", Type: "bool", Description: "Include sha256 for regular files", Default: false},
			},
			Handler: func(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
				return handleList(input)
			},
		},
		{
			Name:        "stat",
			Description: "Get file metadata",
			Args: []toolkit.Arg{
				{Name: "path", Description: "File path", Required: true},
			},
			Flags: []toolkit.Flag{
				{Name: "include-hash", Type: "bool", Description: "Include sha256 for regular files", Default: true},
			},
			Handler: func(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
				return handleStat(input)
			},
		},
		{
			Name:        "rm",
			Description: "Remove a file or directory",
			Args: []toolkit.Arg{
				{Name: "path", Description: "File or directory path", Required: true},
			},
			Flags: []toolkit.Flag{
				{Name: "approved", Type: "bool", Description: "Must be true after explicit user approval", Default: false},
			},
			Handler: func(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
				if out, blocked := requireMutationApproval(input); blocked {
					return out, nil
				}
				err := os.RemoveAll(input.Args["path"])
				if err != nil {
					return toolkit.Output{Error: err.Error()}, nil
				}
				return toolkit.Output{}, nil
			},
		},
	}
}

type fileEntry struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	Mode         string `json:"mode"`
	Modified     string `json:"modified"`
	IsDir        bool   `json:"is_dir"`
	SHA256       string `json:"sha256,omitempty"`
}

func handleList(input toolkit.Input) (toolkit.Output, error) {
	root := input.Args["path"]
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	recursive, err := boolFlag(input.Flags, "recursive", false)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	includeHash, err := boolFlag(input.Flags, "include-hash", false)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	entries, err := listEntries(root, recursive, includeHash)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return toolkit.Output{Data: map[string]any{
		"entries": names,
		"files":   entries,
	}}, nil
}

func handleStat(input toolkit.Input) (toolkit.Output, error) {
	path := input.Args["path"]
	includeHash, err := boolFlag(input.Flags, "include-hash", true)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	entry, err := fileInfoEntry(filepath.Dir(path), path, info, includeHash)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	return toolkit.Output{Data: map[string]any{
		"name":          entry.Name,
		"path":          entry.Path,
		"relative_path": entry.RelativePath,
		"size":          entry.Size,
		"mode":          entry.Mode,
		"modified":      entry.Modified,
		"is_dir":        entry.IsDir,
		"sha256":        entry.SHA256,
	}}, nil
}

func listEntries(root string, recursive, includeHash bool) ([]fileEntry, error) {
	if !recursive {
		dirEntries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		out := make([]fileEntry, 0, len(dirEntries))
		for _, dirEntry := range dirEntries {
			path := filepath.Join(root, dirEntry.Name())
			info, err := dirEntry.Info()
			if err != nil {
				return nil, err
			}
			entry, err := fileInfoEntry(root, path, info, includeHash)
			if err != nil {
				return nil, err
			}
			out = append(out, entry)
		}
		return out, nil
	}

	out := make([]fileEntry, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry, err := fileInfoEntry(root, path, info, includeHash)
		if err != nil {
			return err
		}
		out = append(out, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func fileInfoEntry(root, path string, info os.FileInfo, includeHash bool) (fileEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fileEntry{}, err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		relative = info.Name()
	}
	entry := fileEntry{
		Name:         info.Name(),
		Path:         absPath,
		RelativePath: filepath.ToSlash(relative),
		Size:         info.Size(),
		Mode:         info.Mode().String(),
		Modified:     info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		IsDir:        info.IsDir(),
	}
	if includeHash && info.Mode().IsRegular() {
		hash, err := fileSHA256(path)
		if err != nil {
			return fileEntry{}, err
		}
		entry.SHA256 = hash
	}
	return entry, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func handleExtractPDF(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
	path := input.Args["path"]
	if strings.TrimSpace(path) == "" {
		return toolkit.Output{Error: "missing required argument: path"}, nil
	}

	maxChars, err := intFlag(input.Flags, "max-chars", defaultPDFMaxChars)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}

	cmd := exec.CommandContext(ctx, "pdftotext", path, "-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return toolkit.Output{Error: fmt.Sprintf("pdftotext %s: %v: %s", path, err, msg)}, nil
		}
		return toolkit.Output{Error: fmt.Sprintf("pdftotext %s: %v", path, err)}, nil
	}

	content := strings.TrimSpace(string(out))
	runes := []rune(content)
	originalChars := len(runes)
	truncated := false
	if maxChars > 0 && originalChars > maxChars {
		content = string(runes[:maxChars])
		truncated = true
	}

	return toolkit.Output{Data: map[string]any{
		"content":        content,
		"chars":          len([]rune(content)),
		"original_chars": originalChars,
		"truncated":      truncated,
	}}, nil
}

func handleRead(input toolkit.Input) (toolkit.Output, error) {
	path := input.Args["path"]
	data, err := os.ReadFile(path)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	content := string(data)
	startLine := 0
	endLine := 0
	if v, ok := input.Flags["start-line"]; ok {
		startLine = v.(int)
	}
	if v, ok := input.Flags["end-line"]; ok {
		endLine = v.(int)
	}
	if startLine > 0 || endLine > 0 {
		lines := strings.Split(content, "\n")
		total := len(lines)
		if startLine <= 0 {
			startLine = 1
		}
		if endLine <= 0 || endLine > total {
			endLine = total
		}
		if startLine > total {
			startLine = total
		}
		var selected []string
		for i := startLine - 1; i < endLine && i < total; i++ {
			selected = append(selected, lines[i])
		}
		content = strings.Join(selected, "\n")
		return toolkit.Output{Data: map[string]any{
			"content":     content,
			"total_lines": total,
			"start_line":  startLine,
			"end_line":    endLine,
		}}, nil
	}
	return toolkit.Output{Data: map[string]any{"content": content}}, nil
}

func requireMutationApproval(input toolkit.Input) (toolkit.Output, bool) {
	approved, err := boolFlag(input.Flags, "approved", false)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, true
	}
	if !approved {
		return toolkit.Output{Error: mutationApprovalRequired}, true
	}
	return toolkit.Output{}, false
}

func intFlag(flags map[string]any, name string, fallback int) (int, error) {
	value, ok := flagValue(flags, name)
	if !ok || value == nil {
		return fallback, nil
	}
	switch v := value.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("flag %s must be an int", name)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("flag %s must be an int", name)
	}
}

func boolFlag(flags map[string]any, name string, fallback bool) (bool, error) {
	value, ok := flagValue(flags, name)
	if !ok || value == nil {
		return fallback, nil
	}
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback, nil
		}
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("flag %s must be a bool", name)
		}
		return parsed, nil
	default:
		return false, fmt.Errorf("flag %s must be a bool", name)
	}
}

func flagValue(flags map[string]any, name string) (any, bool) {
	for _, candidate := range []string{name, strings.ReplaceAll(name, "-", "_"), strings.ReplaceAll(name, "_", "-")} {
		if v, ok := flags[candidate]; ok {
			return v, true
		}
	}
	return nil, false
}
