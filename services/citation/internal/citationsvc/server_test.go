package citationsvc

import (
	"context"
	"strings"
	"testing"

	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
)

func TestResolveSpansFindsExactAndNormalizedEvidence(t *testing.T) {
	t.Parallel()
	server := NewServer()
	resp, err := server.ResolveSpans(context.Background(), &citationv1.ResolveSpansRequest{
		SourceUri:  "paper.pdf",
		SourceText: "The Transformer uses multi-head\nself-attention for sequence modelling.",
		Queries: []*citationv1.SpanQuery{{
			Id:   "exact",
			Text: "Transformer uses multi-head",
		}, {
			Id:   "normalized",
			Text: "multi-head self-attention",
		}, {
			Id:   "missing",
			Text: "convolutional recurrence only",
		}},
	})
	if err != nil {
		t.Fatalf("resolve spans: %v", err)
	}
	if got := len(resp.GetSpans()); got != 2 {
		t.Fatalf("spans = %d, want 2: %+v", got, resp.GetSpans())
	}
	if resp.GetSpans()[0].GetId() != "exact" || resp.GetSpans()[0].GetConfidence() != exactConfidence {
		t.Fatalf("exact span mismatch: %+v", resp.GetSpans()[0])
	}
	if resp.GetSpans()[1].GetId() != "normalized" || !strings.Contains(resp.GetSpans()[1].GetTextSpan(), "\n") {
		t.Fatalf("normalized span mismatch: %+v", resp.GetSpans()[1])
	}
}

func TestResolveSpansRejectsMissingSourceText(t *testing.T) {
	t.Parallel()
	_, err := NewServer().ResolveSpans(context.Background(), &citationv1.ResolveSpansRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCreateCitationReturnsNotFoundForUnsupportedSourceEvidence(t *testing.T) {
	t.Parallel()
	_, err := NewServer().CreateCitation(context.Background(), &citationv1.CreateCitationRequest{
		SourceText: "source",
		Text:       "missing",
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestVerifyGroundingScoresClaimsAgainstCitationSpans(t *testing.T) {
	t.Parallel()
	resp, err := NewServer().VerifyGrounding(context.Background(), &citationv1.VerifyGroundingRequest{
		Claims: []*citationv1.GroundedClaim{{
			Id:    "grounded",
			Claim: "The Transformer uses self-attention for sequence modelling.",
			Citations: []*citationv1.CitationSpan{{
				TextSpan: "Transformer uses multi-head self-attention for sequence modelling.",
			}},
		}, {
			Id:    "ungrounded",
			Claim: "The receipt was paid in cash.",
			Citations: []*citationv1.CitationSpan{{
				TextSpan: "The invoice total is EUR 18,450.00.",
			}},
		}, {
			Id:    "missing-span",
			Claim: "The contract has a four hour response target.",
		}},
	})
	if err != nil {
		t.Fatalf("verify grounding: %v", err)
	}
	if len(resp.GetResults()) != 3 {
		t.Fatalf("results = %+v, want 3", resp.GetResults())
	}
	if !resp.GetResults()[0].GetGrounded() {
		t.Fatalf("first claim should be grounded: %+v", resp.GetResults()[0])
	}
	if resp.GetResults()[1].GetGrounded() || resp.GetResults()[2].GetGrounded() {
		t.Fatalf("ungrounded claims passed: %+v", resp.GetResults())
	}
}

func TestScoreCoverageAndRenderReferences(t *testing.T) {
	t.Parallel()
	server := NewServer()
	coverage, err := server.ScoreCoverage(context.Background(), &citationv1.ScoreCoverageRequest{
		Claims: []*citationv1.GroundedClaim{{
			Id:    "claim-1",
			Claim: "Quark stores indexed context.",
			Citations: []*citationv1.CitationSpan{{
				TextSpan: "Quark stores indexed context for retrieval.",
			}},
		}, {
			Id:    "claim-2",
			Claim: "Quark deploys satellites.",
			Citations: []*citationv1.CitationSpan{{
				TextSpan: "Quark stores indexed context for retrieval.",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("score coverage: %v", err)
	}
	if coverage.GetGroundedCount() != 1 || coverage.GetTotalCount() != 2 || coverage.GetCoverage() != 0.5 {
		t.Fatalf("coverage = %+v, want one of two grounded", coverage)
	}

	rendered, err := server.RenderReferences(context.Background(), &citationv1.RenderReferencesRequest{
		Citations: []*citationv1.CitationSpan{{
			Id:          "cite-1",
			SourceUri:   "paper.pdf",
			TextSpan:    "Transformer uses attention.",
			StartOffset: 10,
			EndOffset:   37,
		}},
	})
	if err != nil {
		t.Fatalf("render references: %v", err)
	}
	if len(rendered.GetReferences()) != 1 || !strings.Contains(rendered.GetMarkdown(), "paper.pdf:10-37") {
		t.Fatalf("rendered references = %+v", rendered)
	}
}
