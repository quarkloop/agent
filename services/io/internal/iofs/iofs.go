package iofs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	DefaultPDFMaxChars       = 30000
	MutationApprovalRequired = "workspace mutation requires explicit user approval"
)

var ErrMutationNotApproved = errors.New(MutationApprovalRequired)
var ErrInvalidPath = errors.New("path is required")

type FileEntry struct {
	Name         string
	Path         string
	RelativePath string
	Size         int64
	Mode         string
	Modified     string
	IsDir        bool
	SHA256       string
}

type ReadResult struct {
	Content    string
	TotalLines int32
	StartLine  int32
	EndLine    int32
}

type ListResult struct {
	Entries []string
	Files   []FileEntry
}

type ExtractPdfResult struct {
	Content       string
	Chars         int32
	OriginalChars int32
	Truncated     bool
}

func Read(path string, startLine, endLine int32) (ReadResult, error) {
	path, err := cleanPath(path)
	if err != nil {
		return ReadResult{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReadResult{}, err
	}
	content := string(data)
	if startLine > 0 || endLine > 0 {
		lines := strings.Split(content, "\n")
		total := len(lines)
		start := int(startLine)
		end := int(endLine)
		if start <= 0 {
			start = 1
		}
		if end <= 0 || end > total {
			end = total
		}
		if start > total {
			start = total
		}
		var selected []string
		for i := start - 1; i < end && i < total; i++ {
			selected = append(selected, lines[i])
		}
		content = strings.Join(selected, "\n")
		return ReadResult{
			Content:    content,
			TotalLines: int32(total),
			StartLine:  int32(start),
			EndLine:    int32(end),
		}, nil
	}
	return ReadResult{Content: content}, nil
}

func List(path string, recursive, includeHash bool) (ListResult, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	path, err := cleanPath(path)
	if err != nil {
		return ListResult{}, err
	}
	entries, err := listEntries(path, recursive, includeHash)
	if err != nil {
		return ListResult{}, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return ListResult{Entries: names, Files: entries}, nil
}

func Stat(path string, includeHash bool) (FileEntry, error) {
	path, err := cleanPath(path)
	if err != nil {
		return FileEntry{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return FileEntry{}, err
	}
	return fileInfoEntry(filepath.Dir(path), path, info, includeHash)
}

func Write(path, content string, approved bool) (int, error) {
	if err := requireApproved(approved); err != nil {
		return 0, err
	}
	path, err := cleanMutationPath(path)
	if err != nil {
		return 0, err
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return 0, err
	}
	return len(content), nil
}

func Append(path, content string, approved bool) (int, error) {
	if err := requireApproved(approved); err != nil {
		return 0, err
	}
	path, err := cleanMutationPath(path)
	if err != nil {
		return 0, err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	next := make([]byte, 0, len(existing)+len(content))
	next = append(next, existing...)
	next = append(next, content...)
	if err := atomicWriteFile(path, next, 0o644); err != nil {
		return 0, err
	}
	return len(content), nil
}

func Replace(path, find, replaceWith string, approved bool) (int, error) {
	if err := requireApproved(approved); err != nil {
		return 0, err
	}
	path, err := cleanMutationPath(path)
	if err != nil {
		return 0, err
	}
	if find == "" {
		return 0, errors.New("find is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	replacements := strings.Count(string(data), find)
	if replacements == 0 {
		return 0, nil
	}
	newContent := strings.ReplaceAll(string(data), find, replaceWith)
	if err := atomicWriteFile(path, []byte(newContent), 0o644); err != nil {
		return 0, err
	}
	return replacements, nil
}

func Remove(path string, approved bool) error {
	if err := requireApproved(approved); err != nil {
		return err
	}
	path, err := cleanMutationPath(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func ExtractPdf(ctx context.Context, path string, maxChars int32, pdftotext string) (ExtractPdfResult, error) {
	path, err := cleanPath(path)
	if err != nil {
		return ExtractPdfResult{}, err
	}
	if maxChars <= 0 {
		maxChars = DefaultPDFMaxChars
	}
	bin := pdftotext
	if strings.TrimSpace(bin) == "" {
		bin = "pdftotext"
	}
	cmd := exec.CommandContext(ctx, bin, path, "-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return ExtractPdfResult{}, fmt.Errorf("pdftotext %s: %w: %s", path, err, msg)
		}
		return ExtractPdfResult{}, fmt.Errorf("pdftotext %s: %w", path, err)
	}
	content := strings.TrimSpace(string(out))
	runes := []rune(content)
	original := int32(len(runes))
	truncated := false
	if maxChars > 0 && original > maxChars {
		content = string(runes[:maxChars])
		truncated = true
	}
	return ExtractPdfResult{
		Content:       content,
		Chars:         int32(len([]rune(content))),
		OriginalChars: original,
		Truncated:     truncated,
	}, nil
}

func requireApproved(approved bool) error {
	if !approved {
		return ErrMutationNotApproved
	}
	return nil
}

func cleanPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", ErrInvalidPath
	}
	if strings.ContainsRune(path, 0) {
		return "", errors.New("path contains NUL byte")
	}
	return filepath.Clean(path), nil
}

func cleanMutationPath(path string) (string, error) {
	cleaned, err := cleanPath(path)
	if err != nil {
		return "", err
	}
	if cleaned == "." || isRootPath(cleaned) {
		return "", fmt.Errorf("refusing to mutate unsafe path %q", cleaned)
	}
	return cleaned, nil
}

func isRootPath(path string) bool {
	if path == string(filepath.Separator) {
		return true
	}
	volume := filepath.VolumeName(path)
	return volume != "" && filepath.Clean(path) == volume+string(filepath.Separator)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func listEntries(root string, recursive, includeHash bool) ([]FileEntry, error) {
	if !recursive {
		dirEntries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		out := make([]FileEntry, 0, len(dirEntries))
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
	out := make([]FileEntry, 0)
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
	return out, err
}

func fileInfoEntry(root, path string, info os.FileInfo, includeHash bool) (FileEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return FileEntry{}, err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		relative = info.Name()
	}
	entry := FileEntry{
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
			return FileEntry{}, err
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
