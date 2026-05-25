package extraction

import (
	"strings"
	"testing"
)

func TestRegistryClassifiesConcreteProfiles(t *testing.T) {
	t.Parallel()

	registry := DefaultRegistry()
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "receipt", text: "Invoice total VAT tax merchant amount due", want: "receipt"},
		{name: "cv", text: "Resume candidate experience education skills employment", want: "cv"},
		{name: "paper", text: "Abstract method experiment references transformer attention", want: "paper"},
		{name: "catalog", text: "Catalog category product features pricing brochure", want: "catalog"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			match := registry.Classify(tt.text)
			if match.Profile.ID != tt.want || match.Fallback || match.Confidence <= 0 {
				t.Fatalf("match = %+v, want concrete %s", match, tt.want)
			}
		})
	}
}

func TestRegistryFallsBackToOpenProfile(t *testing.T) {
	t.Parallel()

	match := DefaultRegistry().Classify("A notarized certificate with unusual layout and partial handwritten notes.")
	if match.Profile.ID != "generic-open" || !match.Fallback {
		t.Fatalf("match = %+v, want generic fallback", match)
	}
}

func TestPromptBlockDescribesCanonicalMappingAndValidation(t *testing.T) {
	t.Parallel()

	block := DefaultRegistry().PromptBlock()
	for _, want := range []string{
		"Runtime Extraction Profiles",
		"`generic-open`",
		"UpsertChunkRequest.facts",
		"UpsertChunkRequest.citations",
		"validate",
		"Parser output is raw source evidence",
		"document classification",
		"Do not move this work",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("prompt block missing %q:\n%s", want, block)
		}
	}
}

func TestValidateExtractedRecordRequiresEvidenceAndConsistentRelations(t *testing.T) {
	t.Parallel()

	valid := ExtractedRecord{
		ProfileID:           "paper",
		SourceTextAvailable: true,
		Citations:           []ExtractedCitation{{ID: "cite-1", SourceURI: "paper.pdf"}},
		Entities: []ExtractedEntity{
			{ID: "transformer", Name: "Transformer", Type: "MODEL"},
			{ID: "attention", Name: "Attention", Type: "METHOD"},
		},
		Relations: []ExtractedRelation{{FromID: "transformer", ToID: "attention", Relation: "USES"}},
		Facts:     []ExtractedFact{{Subject: "Transformer", Predicate: "uses", Object: "attention", CitationIDs: []string{"cite-1"}}},
	}
	if err := ValidateExtractedRecord(valid); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}

	withoutCitation := valid
	withoutCitation.Facts = []ExtractedFact{{Subject: "Transformer", Predicate: "uses", Object: "attention"}}
	if err := ValidateExtractedRecord(withoutCitation); err == nil {
		t.Fatal("expected missing citation to fail")
	}

	unknownRelationEndpoint := valid
	unknownRelationEndpoint.Relations = []ExtractedRelation{{FromID: "transformer", ToID: "missing", Relation: "USES"}}
	if err := ValidateExtractedRecord(unknownRelationEndpoint); err == nil {
		t.Fatal("expected unknown relation endpoint to fail")
	}
}
