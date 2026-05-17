package docsvc

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

type parser struct {
	pdf pdfExtractor
}

func newParser(pdf pdfExtractor) parser {
	return parser{pdf: pdf}
}

func (p parser) parse(ctx context.Context, source sourceDocument) (parsedDocument, error) {
	if len(source.Content) == 0 {
		return parsedDocument{}, errEmptyInput
	}
	detected := detectSource(source)
	hash := sourceHash(source.Content)
	result := parsedDocument{
		DocumentID: documentID(hash),
		SourceHash: hash,
		MIMEType:   detected.MIMEType,
		Family:     detected.Family,
		Metadata:   cloneMap(detected.Metadata),
		Images:     imagesForSource(source, detected),
	}

	switch detected.Family {
	case "pdf":
		text, err := p.pdf.ExtractText(ctx, source)
		if err != nil {
			return parsedDocument{}, err
		}
		result.Pages = pagesFromPDFText(text)
		result.TextAvailable = len(strings.TrimSpace(text)) > 0
	case "markdown", "text":
		if !utf8.Valid(source.Content) {
			return parsedDocument{}, fmt.Errorf("%w: text content is not valid UTF-8", errUnsupportedType)
		}
		result.Pages = []textPage{{PageNumber: 1, Text: string(source.Content)}}
		result.TextAvailable = len(strings.TrimSpace(string(source.Content))) > 0
	case "image":
		result.PageCount = 1
		result.TextAvailable = false
		result.Layouts = []layoutPage{{PageNumber: 1, Width: 1, Height: 1}}
		return result, nil
	default:
		return parsedDocument{}, errUnsupportedType
	}

	result.PageCount = int32(len(result.Pages))
	result.Pages = withOffsets(result.Pages)
	result.Tables = tablesFromPages(result.Pages)
	result.Layouts = layoutsFromPages(result.Pages)
	return result, nil
}

func pagesFromPDFText(text string) []textPage {
	parts := strings.Split(text, "\f")
	pages := make([]textPage, 0, len(parts))
	for _, part := range parts {
		cleaned := strings.TrimSpace(part)
		if cleaned == "" && len(parts) > 1 {
			continue
		}
		pages = append(pages, textPage{PageNumber: int32(len(pages) + 1), Text: cleaned})
	}
	if len(pages) == 0 && strings.TrimSpace(text) != "" {
		pages = append(pages, textPage{PageNumber: 1, Text: strings.TrimSpace(text)})
	}
	return pages
}

func withOffsets(pages []textPage) []textPage {
	out := make([]textPage, 0, len(pages))
	offset := 0
	for i, page := range pages {
		if page.PageNumber == 0 {
			page.PageNumber = int32(i + 1)
		}
		page.StartOffset = int32(offset)
		offset += len(page.Text)
		page.EndOffset = int32(offset)
		if i < len(pages)-1 {
			offset += 2
		}
		out = append(out, page)
	}
	return out
}

func joinedText(pages []textPage) string {
	parts := make([]string, 0, len(pages))
	for _, page := range pages {
		parts = append(parts, page.Text)
	}
	return strings.Join(parts, "\n\n")
}

func limitedTextPages(pages []textPage, maxChars int32) (string, []textPage) {
	text := joinedText(pages)
	if maxChars <= 0 || len([]rune(text)) <= int(maxChars) {
		return text, pages
	}
	limited := string([]rune(text)[:int(maxChars)])
	limit := len(limited)
	out := make([]textPage, 0, len(pages))
	for _, page := range pages {
		if int(page.StartOffset) >= limit {
			break
		}
		next := page
		if int(next.EndOffset) > limit {
			next.Text = text[int(page.StartOffset):limit]
			next.EndOffset = int32(limit)
		}
		out = append(out, next)
	}
	return limited, out
}
