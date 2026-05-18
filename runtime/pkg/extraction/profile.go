package extraction

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Profile struct {
	ID               string
	Name             string
	Description      string
	DocumentHints    []string
	RequiredSections []string
	CanonicalMapping []string
}

type Match struct {
	Profile    Profile
	Confidence float32
	Fallback   bool
	Reasons    []string
}

type Registry struct {
	profiles []Profile
	fallback Profile
}

type ExtractedRecord struct {
	ProfileID           string
	SourceTextAvailable bool
	Facts               []ExtractedFact
	Entities            []ExtractedEntity
	Relations           []ExtractedRelation
	Citations           []ExtractedCitation
}

type ExtractedFact struct {
	Subject     string
	Predicate   string
	Object      string
	CitationIDs []string
}

type ExtractedEntity struct {
	ID   string
	Name string
	Type string
}

type ExtractedRelation struct {
	FromID   string
	ToID     string
	Relation string
}

type ExtractedCitation struct {
	ID        string
	SourceURI string
	TextSpan  string
}

func DefaultRegistry() Registry {
	fallback := Profile{
		ID:          "generic-open",
		Name:        "Generic open extraction",
		Description: "Use for unknown or mixed documents. Extract summary, key facts, entities, relations, citations, sections, tables, dates, amounts, and uncertainties without requiring a concrete schema.",
		DocumentHints: []string{
			"unknown", "mixed", "manual", "certificate", "report", "book", "letter",
		},
		RequiredSections: []string{
			"document_type_guess", "summary", "key_facts", "entities", "relations", "citations", "uncertainties",
		},
		CanonicalMapping: []string{
			"document -> IndexRequest.document",
			"summary/key sections -> IndexRequest.textContent",
			"key_facts -> IndexRequest.facts",
			"entities -> IndexRequest.entities",
			"relations -> IndexRequest.relations",
			"citations -> IndexRequest.citations",
			"source evidence -> IndexRequest.provenance and sourceMetadata",
		},
	}
	return Registry{
		fallback: fallback,
		profiles: []Profile{
			{
				ID:          "receipt",
				Name:        "Receipt or invoice",
				Description: "Use for receipts, invoices, bills, and purchase confirmations.",
				DocumentHints: []string{
					"receipt", "invoice", "total", "subtotal", "tax", "vat", "merchant", "amount due", "payment",
				},
				RequiredSections: []string{
					"merchant", "date", "line_items", "totals", "taxes", "currency", "payment_status", "citations",
				},
				CanonicalMapping: []string{
					"merchant/date/totals -> facts",
					"merchant and billed parties -> entities",
					"line items -> facts with citations",
					"source path/hash -> provenance",
				},
			},
			{
				ID:          "cv",
				Name:        "CV or resume",
				Description: "Use for resumes, CVs, bios, and candidate profiles.",
				DocumentHints: []string{
					"resume", "curriculum vitae", "cv", "experience", "education", "skills", "candidate", "employment",
				},
				RequiredSections: []string{
					"candidate", "current_or_recent_role", "experience", "education", "skills", "contact", "citations",
				},
				CanonicalMapping: []string{
					"candidate and employers -> entities",
					"roles/skills/education -> facts",
					"employment relations -> relations",
					"source path/hash -> provenance",
				},
			},
			{
				ID:          "paper",
				Name:        "Research paper",
				Description: "Use for academic papers, preprints, technical reports, and scientific articles.",
				DocumentHints: []string{
					"abstract", "introduction", "method", "experiment", "references", "paper", "attention", "transformer",
				},
				RequiredSections: []string{
					"title", "authors", "abstract", "core_claims", "methods", "results", "limitations", "citations",
				},
				CanonicalMapping: []string{
					"title/authors -> document metadata and entities",
					"claims/results/limitations -> facts",
					"concept relationships -> relations",
					"source path/hash -> provenance",
				},
			},
			{
				ID:          "catalog",
				Name:        "Catalog or brochure",
				Description: "Use for catalogs, brochures, menus, idea lists, product sheets, and service listings.",
				DocumentHints: []string{
					"catalog", "brochure", "product", "service", "category", "idea", "features", "pricing",
				},
				RequiredSections: []string{
					"categories", "items", "descriptions", "attributes", "audience", "citations",
				},
				CanonicalMapping: []string{
					"categories/items -> entities",
					"item attributes and descriptions -> facts",
					"category membership -> relations",
					"source path/hash -> provenance",
				},
			},
		},
	}
}

func (r Registry) Profiles() []Profile {
	out := make([]Profile, len(r.profiles))
	copy(out, r.profiles)
	return out
}

func (r Registry) Fallback() Profile {
	return r.fallback
}

func (r Registry) Classify(text string) Match {
	text = strings.ToLower(text)
	best := Match{Profile: r.fallback, Fallback: true}
	for _, profile := range r.profiles {
		score, reasons := scoreProfile(text, profile)
		if score > best.Confidence {
			best = Match{
				Profile:    profile,
				Confidence: score,
				Reasons:    reasons,
			}
		}
	}
	if best.Confidence < 0.35 {
		best = Match{
			Profile:    r.fallback,
			Confidence: 0.25,
			Fallback:   true,
			Reasons:    []string{"no concrete profile reached the confidence threshold"},
		}
	}
	return best
}

func (r Registry) PromptBlock() string {
	var b strings.Builder
	b.WriteString("## Runtime Extraction Profiles\n\n")
	b.WriteString("When indexing documents, choose an extraction profile in the agent's runtime/model LLM path before mapping extracted content into the indexer canonical schema. Parser output is raw source evidence; semantic extraction output is separate and must be validated before indexing.\n")
	b.WriteString("The LLM-owned work is document classification, schema inference, field normalization, chunk decisions, fact extraction, entity extraction, relation extraction, and citation selection. Do not move this work into document, ingestion, citation, memory, embedding, or indexer services.\n")
	b.WriteString("If no concrete profile fits confidently, use `generic-open` and preserve uncertainty instead of forcing a schema.\n")
	b.WriteString("The open extraction structure supports document_type_guess, summary, key_facts, entities, relations, dates, amounts, tables, sections, citations, raw_text_references, and uncertainties. Keep raw text references separate from semantic facts.\n")

	profiles := append(r.Profiles(), r.fallback)
	sort.SliceStable(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	for _, profile := range profiles {
		fmt.Fprintf(&b, "\n### `%s` - %s\n\n", profile.ID, profile.Name)
		fmt.Fprintf(&b, "%s\n", profile.Description)
		if len(profile.DocumentHints) > 0 {
			fmt.Fprintf(&b, "- Hints: %s\n", strings.Join(profile.DocumentHints, ", "))
		}
		if len(profile.RequiredSections) > 0 {
			fmt.Fprintf(&b, "- Required extraction sections: %s\n", strings.Join(profile.RequiredSections, ", "))
		}
		if len(profile.CanonicalMapping) > 0 {
			b.WriteString("- Canonical mapping:\n")
			for _, mapping := range profile.CanonicalMapping {
				fmt.Fprintf(&b, "  - %s\n", mapping)
			}
		}
	}
	b.WriteString("\nBefore calling `indexer_UpsertChunk`, validate that extracted facts have evidence when source text exists, entities and relations are internally consistent, and unsupported partial fields are either omitted or marked uncertain.\n")
	return strings.TrimSpace(b.String())
}

func ValidateExtractedRecord(record ExtractedRecord) error {
	citationIDs := make(map[string]struct{}, len(record.Citations))
	for _, citation := range record.Citations {
		id := strings.TrimSpace(citation.ID)
		if id == "" {
			return errors.New("citation id is required")
		}
		if strings.TrimSpace(citation.SourceURI) == "" && strings.TrimSpace(citation.TextSpan) == "" {
			return fmt.Errorf("citation %q requires source_uri or text_span", id)
		}
		citationIDs[id] = struct{}{}
	}

	entityIDs := make(map[string]struct{}, len(record.Entities))
	for _, entity := range record.Entities {
		id := strings.TrimSpace(entity.ID)
		if id == "" {
			return errors.New("entity id is required")
		}
		if strings.TrimSpace(entity.Name) == "" {
			return fmt.Errorf("entity %q requires name", id)
		}
		entityIDs[id] = struct{}{}
	}

	for _, relation := range record.Relations {
		if strings.TrimSpace(relation.FromID) == "" || strings.TrimSpace(relation.ToID) == "" || strings.TrimSpace(relation.Relation) == "" {
			return errors.New("relations require from_id, to_id, and relation")
		}
		if _, ok := entityIDs[relation.FromID]; !ok {
			return fmt.Errorf("relation references unknown from_id %q", relation.FromID)
		}
		if _, ok := entityIDs[relation.ToID]; !ok {
			return fmt.Errorf("relation references unknown to_id %q", relation.ToID)
		}
	}

	for _, fact := range record.Facts {
		if strings.TrimSpace(fact.Subject) == "" || strings.TrimSpace(fact.Predicate) == "" || strings.TrimSpace(fact.Object) == "" {
			return errors.New("facts require subject, predicate, and object")
		}
		if record.SourceTextAvailable && len(fact.CitationIDs) == 0 {
			return fmt.Errorf("fact %q/%q requires citation evidence", fact.Subject, fact.Predicate)
		}
		for _, citationID := range fact.CitationIDs {
			if _, ok := citationIDs[strings.TrimSpace(citationID)]; !ok {
				return fmt.Errorf("fact references unknown citation %q", citationID)
			}
		}
	}
	return nil
}

func scoreProfile(text string, profile Profile) (float32, []string) {
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}
	var hits int
	reasons := make([]string, 0)
	for _, hint := range profile.DocumentHints {
		hint = strings.ToLower(strings.TrimSpace(hint))
		if hint == "" {
			continue
		}
		if strings.Contains(text, hint) {
			hits++
			reasons = append(reasons, hint)
		}
	}
	if hits == 0 {
		return 0, nil
	}
	score := float32(hits) / float32(len(profile.DocumentHints))
	if score < 0.35 {
		score = 0.35
	}
	if score > 1 {
		score = 1
	}
	return score, reasons
}
