package iosvc

import (
	"context"
	"errors"
	"log/slog"
	"os"

	iov1 "github.com/quarkloop/pkg/serviceapi/gen/quark/io/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"github.com/quarkloop/services/io/internal/iofetch"
	"github.com/quarkloop/services/io/internal/iofs"
	"github.com/quarkloop/services/io/internal/iosearch"
	"github.com/quarkloop/services/io/internal/ioshell"
)

type Config struct {
	PDFToText string
	Logger    *slog.Logger
}

type Server struct {
	pdfToText string
	logger    *slog.Logger
}

func NewServer(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Server{pdfToText: cfg.PDFToText, logger: cfg.Logger}
}

func (s *Server) Read(_ context.Context, req *iov1.ReadRequest) (*iov1.ReadResponse, error) {
	result, err := iofs.Read(req.GetPath(), req.GetStartLine(), req.GetEndLine())
	if err != nil {
		return nil, grpcError(err)
	}
	return &iov1.ReadResponse{
		Content:    result.Content,
		TotalLines: result.TotalLines,
		StartLine:  result.StartLine,
		EndLine:    result.EndLine,
	}, nil
}

func (s *Server) List(_ context.Context, req *iov1.ListRequest) (*iov1.ListResponse, error) {
	result, err := iofs.List(req.GetPath(), req.GetRecursive(), req.GetIncludeHash())
	if err != nil {
		return nil, grpcError(err)
	}
	return &iov1.ListResponse{
		Entries: result.Entries,
		Files:   fileEntriesToProto(result.Files),
	}, nil
}

func (s *Server) Stat(_ context.Context, req *iov1.StatRequest) (*iov1.StatResponse, error) {
	includeHash := req.GetIncludeHash()
	entry, err := iofs.Stat(req.GetPath(), includeHash)
	if err != nil {
		return nil, grpcError(err)
	}
	return &iov1.StatResponse{Entry: fileEntryToProto(entry)}, nil
}

func (s *Server) Write(_ context.Context, req *iov1.WriteRequest) (*iov1.WriteResponse, error) {
	n, err := iofs.Write(req.GetPath(), req.GetContent(), req.GetApproved())
	if err != nil {
		return nil, grpcError(err)
	}
	return &iov1.WriteResponse{Written: int32(n)}, nil
}

func (s *Server) Append(_ context.Context, req *iov1.AppendRequest) (*iov1.AppendResponse, error) {
	n, err := iofs.Append(req.GetPath(), req.GetContent(), req.GetApproved())
	if err != nil {
		return nil, grpcError(err)
	}
	return &iov1.AppendResponse{Appended: int32(n)}, nil
}

func (s *Server) Replace(_ context.Context, req *iov1.ReplaceRequest) (*iov1.ReplaceResponse, error) {
	n, err := iofs.Replace(req.GetPath(), req.GetFind(), req.GetReplaceWith(), req.GetApproved())
	if err != nil {
		return nil, grpcError(err)
	}
	return &iov1.ReplaceResponse{Replacements: int32(n)}, nil
}

func (s *Server) Remove(_ context.Context, req *iov1.RemoveRequest) (*iov1.RemoveResponse, error) {
	if err := iofs.Remove(req.GetPath(), req.GetApproved()); err != nil {
		return nil, grpcError(err)
	}
	return &iov1.RemoveResponse{}, nil
}

func (s *Server) ExtractPdf(ctx context.Context, req *iov1.ExtractPdfRequest) (*iov1.ExtractPdfResponse, error) {
	result, err := iofs.ExtractPdf(ctx, req.GetPath(), req.GetMaxChars(), s.pdfToText)
	if err != nil {
		return nil, grpcError(err)
	}
	return &iov1.ExtractPdfResponse{
		Content:       result.Content,
		Chars:         result.Chars,
		OriginalChars: result.OriginalChars,
		Truncated:     result.Truncated,
	}, nil
}

func (s *Server) Execute(_ context.Context, req *iov1.ExecuteRequest) (*iov1.ExecuteResponse, error) {
	output, exitCode, err := ioshell.Execute(req.GetCommand(), req.GetApproved())
	if err != nil {
		return nil, grpcError(err)
	}
	return &iov1.ExecuteResponse{Output: output, ExitCode: exitCode}, nil
}

func (s *Server) SearchWeb(_ context.Context, req *iov1.SearchWebRequest) (*iov1.SearchWebResponse, error) {
	results, query, err := iosearch.Search(req.GetQuery(), int(req.GetMaxResults()))
	if err != nil {
		return nil, grpcError(err)
	}
	out := make([]*iov1.WebResult, 0, len(results))
	for _, r := range results {
		out = append(out, &iov1.WebResult{Title: r.Title, Url: r.URL, Snippet: r.Snippet})
	}
	return &iov1.SearchWebResponse{Query: query, Results: out, Count: int32(len(out))}, nil
}

func (s *Server) Fetch(ctx context.Context, req *iov1.FetchRequest) (*iov1.FetchResponse, error) {
	result := iofetch.Fetch(ctx, req.GetUrl(), req.GetMethod(), req.GetMaxBytes(), req.GetTimeoutSeconds(), req.GetMaxRedirects())
	return &iov1.FetchResponse{
		Url:         result.URL,
		StatusCode:  result.StatusCode,
		ContentType: result.ContentType,
		Body:        result.Body,
		Truncated:   result.Truncated,
		BodyBytes:   result.BodyBytes,
		Error:       result.Error,
	}, nil
}

func fileEntryToProto(entry iofs.FileEntry) *iov1.FileEntry {
	return &iov1.FileEntry{
		Name:         entry.Name,
		Path:         entry.Path,
		RelativePath: entry.RelativePath,
		Size:         entry.Size,
		Mode:         entry.Mode,
		Modified:     entry.Modified,
		IsDir:        entry.IsDir,
		Sha256:       entry.SHA256,
	}
}

func fileEntriesToProto(entries []iofs.FileEntry) []*iov1.FileEntry {
	out := make([]*iov1.FileEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fileEntryToProto(entry))
	}
	return out
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, iofs.ErrMutationNotApproved), errors.Is(err, ioshell.ErrNotApproved):
		return serviceerrors.FailedPrecondition(err.Error())
	case errors.Is(err, os.ErrNotExist):
		return serviceerrors.NotFound(err.Error())
	case errors.Is(err, os.ErrPermission):
		return serviceerrors.PermissionDenied(err.Error())
	default:
		return serviceerrors.InvalidArgument(err.Error())
	}
}
