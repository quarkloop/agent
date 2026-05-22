package workspace

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type SidecarMetadata struct {
	SourcePath        string            `json:"source_path"`
	SourceHash        string            `json:"source_hash,omitempty"`
	DetectedType      string            `json:"detected_type,omitempty"`
	ExtractionProfile string            `json:"extraction_profile,omitempty"`
	Summary           string            `json:"summary,omitempty"`
	Citations         []string          `json:"citations,omitempty"`
	IndexIDs          []string          `json:"index_ids,omitempty"`
	CreatedAt         string            `json:"created_at,omitempty"`
	UpdatedAt         string            `json:"updated_at,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type SourceFile struct {
	Path         string
	SourceHash   string
	DetectedType string
	Title        string
	IndexIDs     []string
}

type SidecarOptions struct {
	Enabled          bool
	Approved         bool
	CreateSidecars   bool
	RestructureFiles bool
}

type PlannedChange struct {
	SourcePath  string
	SidecarPath string
	RenamePath  string
	Metadata    SidecarMetadata
}

type SidecarPlan struct {
	Approved bool
	Changes  []PlannedChange
}

func PlanSidecars(files []SourceFile, opts SidecarOptions) (SidecarPlan, error) {
	if !opts.Enabled || (!opts.CreateSidecars && !opts.RestructureFiles) {
		return SidecarPlan{}, nil
	}
	if !opts.Approved {
		return SidecarPlan{}, fmt.Errorf("sidecar or directory mutation requires explicit user approval")
	}
	changes := make([]PlannedChange, 0, len(files))
	for _, file := range files {
		source := strings.TrimSpace(file.Path)
		if source == "" {
			return SidecarPlan{}, fmt.Errorf("source path is required")
		}
		change := PlannedChange{
			SourcePath: source,
			Metadata: SidecarMetadata{
				SourcePath:   source,
				SourceHash:   strings.TrimSpace(file.SourceHash),
				DetectedType: strings.TrimSpace(file.DetectedType),
				IndexIDs:     append([]string(nil), file.IndexIDs...),
			},
		}
		if opts.CreateSidecars {
			change.SidecarPath = SidecarPath(source)
		}
		if opts.RestructureFiles {
			change.RenamePath = RenamePath(source, file.DetectedType, file.Title)
		}
		changes = append(changes, change)
	}
	return SidecarPlan{Approved: true, Changes: changes}, nil
}

func SidecarPath(sourcePath string) string {
	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+".quark.json")
}

func RenamePath(sourcePath, detectedType, title string) string {
	dir := filepath.Dir(sourcePath)
	ext := filepath.Ext(sourcePath)
	stem := slug(firstNonEmpty(detectedType, "document") + "-" + firstNonEmpty(title, strings.TrimSuffix(filepath.Base(sourcePath), ext)))
	if stem == "" {
		stem = "document"
	}
	return filepath.Join(dir, stem+ext)
}

func PromptBlock() string {
	return strings.TrimSpace(`## Workspace Sidecars And Directory Mutation

Indexing must not depend on sidecar files, file renames, or directory restructuring. Read user files in place and store indexing state in Quark services.

Sidecar metadata files are optional human-facing artifacts. Before creating sidecars, renaming files, or reorganizing a directory, ask the user for explicit approval and present the exact proposed layout. If approval is not present, do not mutate the user's directory.

Deleted or missing sidecars must not break search, retrieval, or re-indexing. Use source path, hash, and modified timestamp from io_Stat or io_List plus indexer provenance as the source of truth.`)
}

var slugUnsafe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugUnsafe.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
