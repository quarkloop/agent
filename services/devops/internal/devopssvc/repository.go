package devopssvc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Server) Status(ctx context.Context, req *devopsv1.StatusRequest) (*devopsv1.StatusResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	branch, _ := s.branchInfo(ctx, root)
	changes, err := s.gitChanges(ctx, root, true)
	if err != nil {
		return nil, operationError(err)
	}
	return &devopsv1.StatusResponse{
		Branch:     branch,
		Clean:      len(changes) == 0,
		Changes:    changes,
		ObservedAt: timestamppb.Now(),
	}, nil
}

func (s *Server) Diff(ctx context.Context, req *devopsv1.DiffRequest) (*devopsv1.DiffResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	files, err := s.workspaces.RelativeFiles(req.GetFiles())
	if err != nil {
		return nil, operationError(err)
	}
	sort.Strings(files)
	args := []string{"diff"}
	if req.GetStaged() {
		args = append(args, "--staged")
	}
	args = append(args, "--")
	args = append(args, files...)
	diff, err := s.commands.Git(ctx, root, args...)
	if err != nil {
		return nil, operationError(err)
	}
	return &devopsv1.DiffResponse{Diff: diff, Files: files}, nil
}

func (s *Server) GetBranch(ctx context.Context, req *devopsv1.GetBranchRequest) (*devopsv1.GetBranchResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	current, upstream := s.branchInfo(ctx, root)
	return &devopsv1.GetBranchResponse{Current: current, Upstream: upstream}, nil
}

func (s *Server) ListChangedFiles(ctx context.Context, req *devopsv1.ListChangedFilesRequest) (*devopsv1.ListChangedFilesResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	changes, err := s.gitChanges(ctx, root, req.GetIncludeUntracked())
	if err != nil {
		return nil, operationError(err)
	}
	return &devopsv1.ListChangedFilesResponse{Changes: changes}, nil
}

func (s *Server) ApplyPatch(ctx context.Context, req *devopsv1.ApplyPatchRequest) (*devopsv1.ApplyPatchResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	if strings.TrimSpace(req.GetPatch()) == "" {
		return nil, serviceerrors.InvalidArgument("patch is required")
	}
	files, err := s.workspaces.RelativeFiles(filesFromPatch(req.GetPatch()))
	if err != nil {
		return nil, operationError(err)
	}
	if req.GetDryRun() {
		if err := s.commands.GitPatchCheck(ctx, root, req.GetPatch()); err != nil {
			return nil, operationError(err)
		}
	}
	return &devopsv1.ApplyPatchResponse{
		Plan:            mutationPlan("repo.apply_patch", root, req.GetReason(), true, "workspace.write"),
		ExpectedChanges: fileChanges(files, "modified"),
	}, nil
}

func (s *Server) Commit(_ context.Context, req *devopsv1.CommitRequest) (*devopsv1.CommitResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	if _, err := s.workspaces.RelativeFiles(req.GetFiles()); err != nil {
		return nil, operationError(err)
	}
	if strings.TrimSpace(req.GetMessage()) == "" {
		return nil, serviceerrors.InvalidArgument("message is required")
	}
	return &devopsv1.CommitResponse{Plan: mutationPlan("repo.commit", root, req.GetReason(), true, "repo.commit")}, nil
}

func (s *Server) GenerateReleaseNotes(ctx context.Context, req *devopsv1.GenerateReleaseNotesRequest) (*devopsv1.GenerateReleaseNotesResponse, error) {
	root, err := s.workspaces.Root(req.GetPath())
	if err != nil {
		return nil, operationError(err)
	}
	rangeRef := strings.TrimSpace(req.GetFromRef())
	toRef := strings.TrimSpace(req.GetToRef())
	if toRef != "" {
		if rangeRef != "" {
			rangeRef += ".."
		}
		rangeRef += toRef
	}
	args := []string{"log", "--pretty=format:%h %s"}
	if rangeRef != "" {
		args = append(args, rangeRef)
	} else {
		args = append(args, "-20")
	}
	out, err := s.commands.Git(ctx, root, args...)
	if err != nil {
		return nil, operationError(err)
	}
	commits := nonEmptyLines(out)
	var b strings.Builder
	b.WriteString("# Release Notes\n")
	for _, commit := range commits {
		fmt.Fprintf(&b, "- %s\n", commit)
	}
	return &devopsv1.GenerateReleaseNotesResponse{Markdown: strings.TrimSpace(b.String()), Commits: commits}, nil
}

func (s *Server) gitChanges(ctx context.Context, root string, includeUntracked bool) ([]*devopsv1.FileChange, error) {
	out, err := s.commands.Git(ctx, root, "status", "--porcelain=v1")
	if err != nil {
		return nil, err
	}
	changes := make([]*devopsv1.FileChange, 0)
	for _, line := range nonEmptyLines(out) {
		if len(line) < 4 {
			continue
		}
		statusCode := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		if strings.HasPrefix(statusCode, "??") && !includeUntracked {
			continue
		}
		changes = append(changes, &devopsv1.FileChange{
			Path:   path,
			Status: gitStatusName(statusCode),
			Staged: line[0] != ' ' && line[0] != '?',
		})
	}
	return changes, nil
}

func (s *Server) branchInfo(ctx context.Context, root string) (string, string) {
	current, _ := s.commands.Git(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	upstream, _ := s.commands.Git(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	return strings.TrimSpace(current), strings.TrimSpace(upstream)
}

func filesFromPatch(patch string) []string {
	seen := map[string]struct{}{}
	for _, line := range strings.Split(patch, "\n") {
		if !strings.HasPrefix(line, "+++ ") && !strings.HasPrefix(line, "--- ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "--- "))
		path = strings.TrimPrefix(path, "a/")
		path = strings.TrimPrefix(path, "b/")
		if path == "/dev/null" || path == "" {
			continue
		}
		seen[path] = struct{}{}
	}
	files := make([]string, 0, len(seen))
	for file := range seen {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

func fileChanges(files []string, status string) []*devopsv1.FileChange {
	changes := make([]*devopsv1.FileChange, 0, len(files))
	for _, file := range files {
		changes = append(changes, &devopsv1.FileChange{Path: file, Status: status})
	}
	return changes
}

func gitStatusName(statusCode string) string {
	switch {
	case strings.Contains(statusCode, "??"):
		return "untracked"
	case strings.Contains(statusCode, "A"):
		return "added"
	case strings.Contains(statusCode, "D"):
		return "deleted"
	case strings.Contains(statusCode, "R"):
		return "renamed"
	case strings.Contains(statusCode, "M"):
		return "modified"
	default:
		return strings.TrimSpace(statusCode)
	}
}
