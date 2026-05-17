package citationsvc

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	exactConfidence           = 1.0
	caseInsensitiveConfidence = 0.9
	normalizedConfidence      = 0.75
	groundedThreshold         = 0.6
)

var wordTokenRE = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9_-]*`)
var unsafeIDRE = regexp.MustCompile(`[^a-z0-9_]+`)

type Server struct {
	citationv1.UnimplementedCitationServiceServer
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) ResolveSpans(_ context.Context, req *citationv1.ResolveSpansRequest) (*citationv1.ResolveSpansResponse, error) {
	sourceText := req.GetSourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil, status.Error(codes.InvalidArgument, "source_text is required")
	}
	spans := make([]*citationv1.CitationSpan, 0, len(req.GetQueries()))
	for _, query := range req.GetQueries() {
		span, ok := resolveQuery(req.GetSourceUri(), sourceText, query.GetId(), query.GetText(), query.GetHint())
		if !ok {
			continue
		}
		spans = append(spans, span)
	}
	return &citationv1.ResolveSpansResponse{Spans: spans}, nil
}

func (s *Server) CreateCitation(_ context.Context, req *citationv1.CreateCitationRequest) (*citationv1.CitationSpan, error) {
	if strings.TrimSpace(req.GetSourceText()) == "" {
		return nil, status.Error(codes.InvalidArgument, "source_text is required")
	}
	span, ok := resolveQuery(req.GetSourceUri(), req.GetSourceText(), req.GetId(), req.GetText(), req.GetHint())
	if !ok {
		return nil, status.Error(codes.NotFound, "citation text was not found in source_text")
	}
	return span, nil
}

func (s *Server) VerifyGrounding(_ context.Context, req *citationv1.VerifyGroundingRequest) (*citationv1.VerifyGroundingResponse, error) {
	return &citationv1.VerifyGroundingResponse{Results: groundingResults(req.GetClaims())}, nil
}

func (s *Server) ScoreCoverage(_ context.Context, req *citationv1.ScoreCoverageRequest) (*citationv1.ScoreCoverageResponse, error) {
	results := groundingResults(req.GetClaims())
	grounded := 0
	for _, result := range results {
		if result.GetGrounded() {
			grounded++
		}
	}
	coverage := float32(0)
	if len(results) > 0 {
		coverage = float32(grounded) / float32(len(results))
	}
	return &citationv1.ScoreCoverageResponse{
		Coverage:      coverage,
		GroundedCount: int32(grounded),
		TotalCount:    int32(len(results)),
		Results:       results,
	}, nil
}

func (s *Server) RenderReferences(_ context.Context, req *citationv1.RenderReferencesRequest) (*citationv1.RenderReferencesResponse, error) {
	citations := append([]*citationv1.CitationSpan(nil), req.GetCitations()...)
	sort.SliceStable(citations, func(i, j int) bool {
		if citations[i].GetSourceUri() == citations[j].GetSourceUri() {
			return citations[i].GetStartOffset() < citations[j].GetStartOffset()
		}
		return citations[i].GetSourceUri() < citations[j].GetSourceUri()
	})
	references := make([]*citationv1.SourceReference, 0, len(citations))
	var b strings.Builder
	for _, citation := range citations {
		if citation == nil {
			continue
		}
		label := fmt.Sprintf("[%d]", len(references)+1)
		reference := &citationv1.SourceReference{
			Id:          firstNonBlank(citation.GetId(), fmt.Sprintf("ref-%d", len(references)+1)),
			SourceUri:   citation.GetSourceUri(),
			TextSpan:    citation.GetTextSpan(),
			StartOffset: citation.GetStartOffset(),
			EndOffset:   citation.GetEndOffset(),
			Label:       label,
		}
		references = append(references, reference)
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s %s", label, reference.GetSourceUri())
		if reference.GetStartOffset() >= 0 || reference.GetEndOffset() > 0 {
			fmt.Fprintf(&b, ":%d-%d", reference.GetStartOffset(), reference.GetEndOffset())
		}
		if strings.TrimSpace(reference.GetTextSpan()) != "" {
			fmt.Fprintf(&b, " - %s", strings.TrimSpace(reference.GetTextSpan()))
		}
	}
	return &citationv1.RenderReferencesResponse{References: references, Markdown: b.String()}, nil
}

func resolveQuery(sourceURI, sourceText, id, text, hint string) (*citationv1.CitationSpan, bool) {
	target := strings.TrimSpace(firstNonBlank(text, hint))
	if target == "" {
		return nil, false
	}
	if span, ok := exactSpan(sourceURI, sourceText, id, target, exactConfidence); ok {
		return span, true
	}
	if span, ok := exactSpan(sourceURI, strings.ToLower(sourceText), id, strings.ToLower(target), caseInsensitiveConfidence); ok {
		raw := sliceRunes(sourceText, int(span.GetStartOffset()), int(span.GetEndOffset()))
		span.TextSpan = raw
		span.SourceUri = sourceURI
		return span, true
	}
	return normalizedSpan(sourceURI, sourceText, id, target)
}

func exactSpan(sourceURI, sourceText, id, target string, confidence float32) (*citationv1.CitationSpan, bool) {
	sourceRunes := []rune(sourceText)
	targetRunes := []rune(target)
	if len(targetRunes) == 0 || len(sourceRunes) < len(targetRunes) {
		return nil, false
	}
	for start := 0; start <= len(sourceRunes)-len(targetRunes); start++ {
		match := true
		for i := range targetRunes {
			if sourceRunes[start+i] != targetRunes[i] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		end := start + len(targetRunes)
		return &citationv1.CitationSpan{
			Id:          firstNonBlank(id, stableCitationID(sourceURI, start, end, sliceRunes(sourceText, start, end))),
			SourceUri:   sourceURI,
			TextSpan:    sliceRunes(sourceText, start, end),
			StartOffset: int32(start),
			EndOffset:   int32(end),
			Confidence:  confidence,
		}, true
	}
	return nil, false
}

func normalizedSpan(sourceURI, sourceText, id, target string) (*citationv1.CitationSpan, bool) {
	source := normalizeWithOffsets(sourceText)
	query := normalizeWithOffsets(target)
	if query.Text == "" {
		return nil, false
	}
	idx := runeIndex([]rune(source.Text), []rune(query.Text))
	if idx < 0 {
		return nil, false
	}
	if idx >= len(source.RawStart) {
		return nil, false
	}
	endIdx := idx + len([]rune(query.Text)) - 1
	if endIdx < 0 || endIdx >= len(source.RawEnd) {
		return nil, false
	}
	start := source.RawStart[idx]
	end := source.RawEnd[endIdx]
	return &citationv1.CitationSpan{
		Id:          firstNonBlank(id, stableCitationID(sourceURI, start, end, sliceRunes(sourceText, start, end))),
		SourceUri:   sourceURI,
		TextSpan:    sliceRunes(sourceText, start, end),
		StartOffset: int32(start),
		EndOffset:   int32(end),
		Confidence:  normalizedConfidence,
	}, true
}

type normalizedText struct {
	Text     string
	RawStart []int
	RawEnd   []int
}

func normalizeWithOffsets(value string) normalizedText {
	var out strings.Builder
	rawStart := make([]int, 0, len(value))
	rawEnd := make([]int, 0, len(value))
	runes := []rune(value)
	inSpace := false
	for i, r := range runes {
		if unicode.IsSpace(r) {
			if out.Len() > 0 && !inSpace {
				out.WriteRune(' ')
				rawStart = append(rawStart, i)
				rawEnd = append(rawEnd, i+1)
				inSpace = true
			}
			continue
		}
		out.WriteRune(unicode.ToLower(r))
		rawStart = append(rawStart, i)
		rawEnd = append(rawEnd, i+1)
		inSpace = false
	}
	text := strings.TrimSpace(out.String())
	if text == out.String() {
		return normalizedText{Text: text, RawStart: rawStart, RawEnd: rawEnd}
	}
	trimmed := strings.TrimLeft(out.String(), " ")
	leftTrim := len([]rune(out.String())) - len([]rune(trimmed))
	rightTrimmed := strings.TrimRight(trimmed, " ")
	return normalizedText{
		Text:     rightTrimmed,
		RawStart: rawStart[leftTrim : leftTrim+len([]rune(rightTrimmed))],
		RawEnd:   rawEnd[leftTrim : leftTrim+len([]rune(rightTrimmed))],
	}
}

func runeIndex(source, target []rune) int {
	if len(target) == 0 || len(source) < len(target) {
		return -1
	}
	for start := 0; start <= len(source)-len(target); start++ {
		match := true
		for i := range target {
			if source[start+i] != target[i] {
				match = false
				break
			}
		}
		if match {
			return start
		}
	}
	return -1
}

func groundingResults(claims []*citationv1.GroundedClaim) []*citationv1.GroundingResult {
	results := make([]*citationv1.GroundingResult, 0, len(claims))
	for _, claim := range claims {
		if claim == nil {
			continue
		}
		result := groundingResult(claim)
		results = append(results, result)
	}
	return results
}

func groundingResult(claim *citationv1.GroundedClaim) *citationv1.GroundingResult {
	id := firstNonBlank(claim.GetId(), stableCitationID("", 0, len([]rune(claim.GetClaim())), claim.GetClaim()))
	if strings.TrimSpace(claim.GetClaim()) == "" {
		return &citationv1.GroundingResult{ClaimId: id, Grounded: false, Confidence: 0, Reason: "claim is empty"}
	}
	if len(claim.GetCitations()) == 0 {
		return &citationv1.GroundingResult{ClaimId: id, Grounded: false, Confidence: 0, Reason: "claim has no citations"}
	}
	citationText := strings.TrimSpace(joinCitationText(claim.GetCitations()))
	if citationText == "" {
		return &citationv1.GroundingResult{ClaimId: id, Grounded: false, Confidence: 0, Reason: "citations have no text spans"}
	}
	tokens := claimTokens(claim.GetClaim())
	if len(tokens) == 0 {
		return &citationv1.GroundingResult{ClaimId: id, Grounded: false, Confidence: 0, Reason: "claim has no meaningful tokens"}
	}
	citationTokens := tokenSet(citationText)
	matched := 0
	for _, token := range tokens {
		if _, ok := citationTokens[token]; ok {
			matched++
		}
	}
	confidence := float32(matched) / float32(len(tokens))
	grounded := confidence >= groundedThreshold
	reason := fmt.Sprintf("%d of %d claim tokens appear in cited text", matched, len(tokens))
	if !grounded {
		reason = "insufficient citation overlap: " + reason
	}
	return &citationv1.GroundingResult{ClaimId: id, Grounded: grounded, Confidence: confidence, Reason: reason}
}

func joinCitationText(citations []*citationv1.CitationSpan) string {
	parts := make([]string, 0, len(citations))
	for _, citation := range citations {
		if citation == nil || strings.TrimSpace(citation.GetTextSpan()) == "" {
			continue
		}
		parts = append(parts, citation.GetTextSpan())
	}
	return strings.Join(parts, "\n")
}

func claimTokens(value string) []string {
	seen := map[string]struct{}{}
	tokens := make([]string, 0)
	for _, token := range wordTokenRE.FindAllString(strings.ToLower(value), -1) {
		if len([]rune(token)) < 3 || stopWord(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}

func tokenSet(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, token := range claimTokens(value) {
		out[token] = struct{}{}
	}
	return out
}

func stopWord(value string) bool {
	switch value {
	case "the", "and", "for", "with", "that", "this", "from", "into", "onto", "are", "was", "were", "has", "have", "had":
		return true
	default:
		return false
	}
}

func stableCitationID(sourceURI string, start, end int, text string) string {
	seed := strings.TrimSpace(sourceURI) + "|" + fmt.Sprint(start) + "|" + fmt.Sprint(end) + "|" + strings.TrimSpace(text)
	seed = strings.ToLower(seed)
	seed = unsafeIDRE.ReplaceAllString(seed, "_")
	seed = strings.Trim(seed, "_")
	if seed == "" {
		return "citation"
	}
	if len(seed) > 80 {
		return seed[:80]
	}
	return seed
}

func sliceRunes(value string, start, end int) string {
	runes := []rune(value)
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start > end {
		start = end
	}
	return string(runes[start:end])
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
