package docsvc

import (
	"context"
	"errors"
	"log/slog"

	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

type Config struct {
	PDFToText string
	Logger    *slog.Logger
}

type Server struct {
	parser parser
}

func NewServer(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	cfg.Logger.Debug("document parser configured")
	return &Server{
		parser: newParser(newCommandPDFExtractor(cfg.PDFToText)),
	}
}

func (s *Server) DetectType(ctx context.Context, req *documentv1.DetectTypeRequest) (*documentv1.DetectTypeResponse, error) {
	input := inputFromProto(req.GetInput())
	source, err := sourceForDetection(ctx, input)
	if err != nil {
		return nil, grpcError(err)
	}
	detected := detectSource(source)
	return &documentv1.DetectTypeResponse{
		MimeType:       detected.MIMEType,
		Extension:      detected.Extension,
		DocumentFamily: detected.Family,
		Confidence:     detected.Confidence,
		Metadata:       cloneMap(detected.Metadata),
	}, nil
}

func (s *Server) ParseBytes(ctx context.Context, req *documentv1.ParseBytesRequest) (*documentv1.ParseBytesResponse, error) {
	parsed, err := s.parse(ctx, req.GetInput())
	if err != nil {
		return nil, grpcError(err)
	}
	return &documentv1.ParseBytesResponse{
		DocumentId:     parsed.DocumentID,
		SourceHash:     parsed.SourceHash,
		MimeType:       parsed.MIMEType,
		DocumentFamily: parsed.Family,
		PageCount:      parsed.PageCount,
		TextAvailable:  parsed.TextAvailable,
		Metadata:       cloneMap(parsed.Metadata),
	}, nil
}

func (s *Server) ExtractText(ctx context.Context, req *documentv1.ExtractTextRequest) (*documentv1.ExtractTextResponse, error) {
	parsed, err := s.parse(ctx, req.GetInput())
	if err != nil {
		return nil, grpcError(err)
	}
	text, pages := limitedTextPages(parsed.Pages, req.GetMaxChars())
	return &documentv1.ExtractTextResponse{
		Text:       text,
		Pages:      pagesToProto(pages, req.GetInput(), parsed),
		SourceHash: parsed.SourceHash,
		Source:     sourceReference(req.GetInput(), parsed, 0, "text"),
	}, nil
}

func (s *Server) ExtractLayout(ctx context.Context, req *documentv1.ExtractLayoutRequest) (*documentv1.ExtractLayoutResponse, error) {
	parsed, err := s.parse(ctx, req.GetInput())
	if err != nil {
		return nil, grpcError(err)
	}
	return &documentv1.ExtractLayoutResponse{Pages: layoutPagesToProto(parsed.Layouts)}, nil
}

func (s *Server) GetPages(ctx context.Context, req *documentv1.GetPagesRequest) (*documentv1.GetPagesResponse, error) {
	parsed, err := s.parse(ctx, req.GetInput())
	if err != nil {
		return nil, grpcError(err)
	}
	pages := make([]*documentv1.Page, 0, len(parsed.Pages))
	layoutByPage := layoutByPageNumber(parsed.Layouts)
	tablesByPage := tablesByPageNumber(parsed.Tables)
	imagesByPage := imagesByPageNumber(parsed.Images)
	for _, page := range parsed.Pages {
		pages = append(pages, &documentv1.Page{
			PageNumber: page.PageNumber,
			Text:       page.Text,
			Blocks:     layoutBlocksToProto(layoutByPage[page.PageNumber]),
			Tables:     tablesToProto(tablesByPage[page.PageNumber]),
			Images:     imagesToProto(imagesByPage[page.PageNumber]),
		})
	}
	if len(pages) == 0 && len(parsed.Images) > 0 {
		pages = append(pages, &documentv1.Page{
			PageNumber: 1,
			Images:     imagesToProto(parsed.Images),
			Blocks:     layoutBlocksToProto(firstLayoutBlocks(parsed.Layouts)),
		})
	}
	return &documentv1.GetPagesResponse{Pages: pages}, nil
}

func (s *Server) ExtractTables(ctx context.Context, req *documentv1.ExtractTablesRequest) (*documentv1.ExtractTablesResponse, error) {
	parsed, err := s.parse(ctx, req.GetInput())
	if err != nil {
		return nil, grpcError(err)
	}
	return &documentv1.ExtractTablesResponse{Tables: tablesToProto(parsed.Tables)}, nil
}

func (s *Server) ExtractImages(ctx context.Context, req *documentv1.ExtractImagesRequest) (*documentv1.ExtractImagesResponse, error) {
	parsed, err := s.parse(ctx, req.GetInput())
	if err != nil {
		return nil, grpcError(err)
	}
	return &documentv1.ExtractImagesResponse{Images: imagesToProto(parsed.Images)}, nil
}

func (s *Server) RunOCR(ctx context.Context, req *documentv1.RunOCRRequest) (*documentv1.RunOCRResponse, error) {
	parsed, err := s.parse(ctx, req.GetInput())
	if err != nil {
		return nil, grpcError(err)
	}
	if parsed.Family == "image" {
		return nil, grpcError(errOCRBackendMissing)
	}
	pages := selectPages(parsed.Pages, req.GetPageNumbers())
	return &documentv1.RunOCRResponse{
		Pages:      pagesToProto(pages, req.GetInput(), parsed),
		Confidence: 1,
		Engine:     "text-layer",
	}, nil
}

func (s *Server) parse(ctx context.Context, input *documentv1.DocumentInput) (parsedDocument, error) {
	source, err := resolveSource(ctx, inputFromProto(input))
	if err != nil {
		return parsedDocument{}, err
	}
	return s.parser.parse(ctx, source)
}

func sourceForDetection(ctx context.Context, input documentInput) (sourceDocument, error) {
	source, err := resolveSource(ctx, input)
	if err == nil {
		return source, nil
	}
	if input.Filename != "" || input.MIMEType != "" {
		return sourceDocument{
			SourceURI: input.SourceURI,
			Filename:  input.Filename,
			MIMEType:  input.MIMEType,
			Metadata:  cloneMap(input.Metadata),
		}, nil
	}
	return sourceDocument{}, err
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, errEmptyInput):
		return serviceerrors.InvalidArgument(err.Error())
	case errors.Is(err, errContentRefOnly):
		return serviceerrors.Unimplemented(err.Error())
	case errors.Is(err, errUnsupportedType):
		return serviceerrors.InvalidArgument(err.Error())
	case errors.Is(err, errPDFBackendMissing):
		return serviceerrors.FailedPrecondition(err.Error())
	case errors.Is(err, errPDFParseFailed):
		return serviceerrors.InvalidArgument(err.Error())
	case errors.Is(err, errOCRBackendMissing):
		return serviceerrors.FailedPrecondition(err.Error())
	default:
		return serviceerrors.Internal(err.Error())
	}
}

func selectPages(pages []textPage, selected []int32) []textPage {
	if len(selected) == 0 {
		out := make([]textPage, len(pages))
		copy(out, pages)
		return out
	}
	want := make(map[int32]struct{}, len(selected))
	for _, pageNumber := range selected {
		if pageNumber > 0 {
			want[pageNumber] = struct{}{}
		}
	}
	out := make([]textPage, 0, len(want))
	for _, page := range pages {
		if _, ok := want[page.PageNumber]; ok {
			out = append(out, page)
		}
	}
	return out
}

func pagesToProto(pages []textPage, input *documentv1.DocumentInput, parsed parsedDocument) []*documentv1.PageText {
	out := make([]*documentv1.PageText, 0, len(pages))
	for _, page := range pages {
		out = append(out, &documentv1.PageText{
			PageNumber:  page.PageNumber,
			Text:        page.Text,
			StartOffset: page.StartOffset,
			EndOffset:   page.EndOffset,
			Source:      sourceReference(input, parsed, page.PageNumber, "text"),
		})
	}
	return out
}

func layoutPagesToProto(pages []layoutPage) []*documentv1.LayoutPage {
	out := make([]*documentv1.LayoutPage, 0, len(pages))
	for _, page := range pages {
		out = append(out, &documentv1.LayoutPage{
			PageNumber: page.PageNumber,
			Width:      page.Width,
			Height:     page.Height,
			Blocks:     layoutBlocksToProto(page.Blocks),
		})
	}
	return out
}

func layoutBlocksToProto(blocks []layoutBlock) []*documentv1.LayoutBlock {
	out := make([]*documentv1.LayoutBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, &documentv1.LayoutBlock{
			Kind: block.Kind,
			Text: block.Text,
			Box:  boxToProto(block.Box),
		})
	}
	return out
}

func tablesToProto(tables []table) []*documentv1.Table {
	out := make([]*documentv1.Table, 0, len(tables))
	for _, item := range tables {
		rows := make([]*documentv1.TableRow, 0, len(item.Rows))
		for _, row := range item.Rows {
			rows = append(rows, &documentv1.TableRow{Cells: append([]string(nil), row.Cells...)})
		}
		out = append(out, &documentv1.Table{
			PageNumber: item.PageNumber,
			Title:      item.Title,
			Headers:    append([]string(nil), item.Headers...),
			Rows:       rows,
			Box:        boxToProto(item.Box),
		})
	}
	return out
}

func imagesToProto(images []image) []*documentv1.Image {
	out := make([]*documentv1.Image, 0, len(images))
	for _, item := range images {
		out = append(out, &documentv1.Image{
			PageNumber: item.PageNumber,
			ImageRef:   item.ImageRef,
			MimeType:   item.MIMEType,
			Box:        boxToProto(item.Box),
			Metadata:   cloneMap(item.Metadata),
			Content:    cloneBytes(item.Content),
			Source: &documentv1.SourceReference{
				SourceUri:  item.SourceURI,
				SourceHash: item.SourceHash,
				MimeType:   item.MIMEType,
				Modality:   "image",
				PageNumber: item.PageNumber,
				Metadata:   cloneMap(item.Metadata),
			},
		})
	}
	return out
}

func sourceReference(input *documentv1.DocumentInput, parsed parsedDocument, pageNumber int32, modality string) *documentv1.SourceReference {
	sourceURI := ""
	artifactRef := ""
	metadata := map[string]string(nil)
	if input != nil {
		sourceURI = input.GetSourceUri()
		artifactRef = input.GetContentRef()
		metadata = cloneMap(input.GetMetadata())
	}
	return &documentv1.SourceReference{
		SourceUri:   sourceURI,
		SourceHash:  parsed.SourceHash,
		MimeType:    parsed.MIMEType,
		Modality:    modality,
		PageNumber:  pageNumber,
		ArtifactRef: artifactRef,
		Metadata:    metadata,
	}
}

func boxToProto(box box) *documentv1.BoundingBox {
	return &documentv1.BoundingBox{
		X:      box.X,
		Y:      box.Y,
		Width:  box.Width,
		Height: box.Height,
	}
}

func layoutByPageNumber(pages []layoutPage) map[int32][]layoutBlock {
	out := make(map[int32][]layoutBlock, len(pages))
	for _, page := range pages {
		out[page.PageNumber] = append([]layoutBlock(nil), page.Blocks...)
	}
	return out
}

func tablesByPageNumber(tables []table) map[int32][]table {
	out := make(map[int32][]table)
	for _, item := range tables {
		out[item.PageNumber] = append(out[item.PageNumber], item)
	}
	return out
}

func imagesByPageNumber(images []image) map[int32][]image {
	out := make(map[int32][]image)
	for _, item := range images {
		out[item.PageNumber] = append(out[item.PageNumber], item)
	}
	return out
}

func firstLayoutBlocks(pages []layoutPage) []layoutBlock {
	if len(pages) == 0 {
		return nil
	}
	return append([]layoutBlock(nil), pages[0].Blocks...)
}
