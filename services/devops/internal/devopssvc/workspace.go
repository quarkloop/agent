package devopssvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type localWorkspace struct{}

func (localWorkspace) Root(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	return filepath.Clean(abs), nil
}

func (localWorkspace) ContainedPath(root, requested, fallback string) (string, error) {
	value := strings.TrimSpace(requested)
	if value == "" {
		value = fallback
	}
	if value == "" {
		return "", fmt.Errorf("relative path is required")
	}
	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("compare workspace path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace", requested)
	}
	return filepath.Clean(candidate), nil
}

func (localWorkspace) RelativeFiles(files []string) ([]string, error) {
	out := make([]string, 0, len(files))
	for _, raw := range files {
		file := strings.TrimSpace(raw)
		if file == "" {
			continue
		}
		clean := filepath.Clean(file)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("file path %q escapes workspace", raw)
		}
		out = append(out, filepath.ToSlash(clean))
	}
	return out, nil
}
