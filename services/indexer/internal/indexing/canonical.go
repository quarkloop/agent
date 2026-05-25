package indexing

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/quarkloop/services/indexer/pkg/indexer"
)

const unknownEntityType = "UNKNOWN"

func knowledgeRecord(cmd IndexCommand) indexer.KnowledgeRecord {
	return indexer.KnowledgeRecord{
		Chunk: indexer.Chunk{
			ID:                cmd.ChunkID,
			Text:              cmd.Text,
			Vector:            cloneVector(cmd.Vector),
			Metadata:          cloneMetadata(cmd.Metadata),
			Document:          cloneDocument(cmd.Document),
			EmbeddingMetadata: cloneEmbeddingMetadata(cmd.EmbeddingMetadata),
			Facts:             cloneFacts(cmd.Facts),
			Citations:         cloneCitations(cmd.Citations),
			Provenance:        cloneProvenance(cmd.Provenance),
		},
		Entities:  primaryEntities(cmd),
		Relations: uniqueRelations(cmd.Relations),
	}
}

func normalizeIndexCommand(cmd IndexCommand) IndexCommand {
	cmd.ChunkID = strings.TrimSpace(cmd.ChunkID)
	cmd.Text = strings.TrimSpace(cmd.Text)
	cmd.Vector = cloneVector(cmd.Vector)
	cmd.Metadata = cloneMetadata(cmd.Metadata)
	cmd.Document = normalizeDocument(cmd.Document, cmd.Metadata, cmd.ChunkID)
	cmd.Provenance = normalizeProvenance(cmd.Provenance, cmd.Metadata, cmd.Document.SourceURI, cmd.Text)
	cmd.Metadata = normalizeSourceMetadata(cmd.Metadata, cmd.Document, cmd.Provenance)
	cmd.Document = normalizeDocument(cmd.Document, cmd.Metadata, cmd.ChunkID)
	cmd.EmbeddingMetadata = normalizeEmbeddingMetadata(cmd.EmbeddingMetadata, cmd.Metadata, len(cmd.Vector))
	cmd.Entities = normalizeEntities(cmd.Entities)
	cmd.Relations = normalizeRelations(cmd.Relations)
	cmd.Citations = normalizeCitations(cmd.Citations, cmd.ChunkID, cmd.Provenance.SourceURI)
	cmd.Facts = normalizeFacts(cmd.Facts, cmd.ChunkID, cmd.Provenance.SourceURI)
	return cmd
}

func validateIndexCommand(cmd IndexCommand) error {
	if cmd.ChunkID == "" {
		return invalid("chunk_id", "is required")
	}
	if cmd.Text == "" {
		return invalid("text_content", "is required")
	}
	if len(cmd.Vector) == 0 {
		return invalid("embedding", "is required")
	}
	if cmd.EmbeddingMetadata.Dimensions > 0 && cmd.EmbeddingMetadata.Dimensions != len(cmd.Vector) {
		return invalid("embedding_metadata.dimensions", fmt.Sprintf("is %d but embedding has %d values", cmd.EmbeddingMetadata.Dimensions, len(cmd.Vector)))
	}
	for _, fact := range cmd.Facts {
		if !validConfidence(fact.Confidence) {
			return invalid("facts.confidence", "must be between 0 and 1")
		}
	}
	for _, citation := range cmd.Citations {
		if !validConfidence(citation.Confidence) {
			return invalid("citations.confidence", "must be between 0 and 1")
		}
		if citation.EndOffset > 0 && citation.StartOffset > citation.EndOffset {
			return invalid("citations.offsets", "start_offset must be less than or equal to end_offset")
		}
	}
	return nil
}

func normalizeEntities(entities []indexer.Entity) []indexer.Entity {
	out := make([]indexer.Entity, 0, len(entities))
	for _, entity := range entities {
		id := strings.TrimSpace(entity.ID)
		name := strings.TrimSpace(entity.Name)
		typ := strings.TrimSpace(entity.Type)
		if id == "" {
			id = indexer.EntityIDFromName(name)
		}
		if name == "" {
			name = id
		}
		if typ == "" {
			typ = unknownEntityType
		}
		if id == "" {
			continue
		}
		out = append(out, indexer.Entity{ID: id, Name: name, Type: typ})
	}
	return out
}

func normalizeRelations(relations []indexer.Relation) []indexer.Relation {
	out := make([]indexer.Relation, 0, len(relations))
	for _, relation := range relations {
		fromID := strings.TrimSpace(relation.FromID)
		toID := strings.TrimSpace(relation.ToID)
		name := strings.TrimSpace(relation.Relation)
		if fromID == "" || toID == "" || name == "" {
			continue
		}
		out = append(out, indexer.Relation{FromID: fromID, ToID: toID, Relation: name})
	}
	return out
}

func normalizeDocument(document indexer.Document, metadata map[string]string, chunkID string) indexer.Document {
	id := strings.TrimSpace(document.ID)
	name := strings.TrimSpace(document.Name)
	typ := strings.TrimSpace(document.Type)
	sourceURI := strings.TrimSpace(document.SourceURI)
	if sourceURI == "" {
		sourceURI = firstMetadata(metadata, "source_uri", "source", "path")
	}
	if name == "" {
		name = firstMetadata(metadata, "filename", "document_name", "source")
	}
	if typ == "" {
		typ = firstMetadata(metadata, "document_type", "type")
	}
	if id == "" {
		id = firstMetadata(metadata, "document_id")
	}
	if id == "" {
		id = indexer.EntityIDFromName(firstNonEmpty(sourceURI, name, chunkID))
	}
	return indexer.Document{
		ID:        id,
		Name:      name,
		Type:      typ,
		SourceURI: sourceURI,
		Metadata:  cloneMetadata(document.Metadata),
		Sources:   normalizeSourceReferences(document.Sources),
	}
}

func normalizeEmbeddingMetadata(embedding indexer.EmbeddingMetadata, metadata map[string]string, vectorLength int) indexer.EmbeddingMetadata {
	provider := strings.TrimSpace(embedding.Provider)
	model := strings.TrimSpace(embedding.Model)
	contentHash := strings.TrimSpace(embedding.ContentHash)
	version := strings.TrimSpace(embedding.Version)
	dimensions := embedding.Dimensions
	if provider == "" {
		provider = firstMetadata(metadata, "embedding_provider", "embeddingProvider")
	}
	if model == "" {
		model = firstMetadata(metadata, "embedding_model", "embeddingModel")
	}
	if contentHash == "" {
		contentHash = firstMetadata(metadata, "embedding_content_hash", "embeddingContentHash", "content_hash", "contentHash")
	}
	if version == "" {
		version = firstMetadata(metadata, "embedding_version", "embeddingVersion")
	}
	if dimensions <= 0 {
		if parsed, ok := parsePositiveInt(firstMetadata(metadata, "embedding_dimensions", "embeddingDimensions", "dimensions")); ok {
			dimensions = parsed
		}
	}
	if dimensions <= 0 {
		dimensions = vectorLength
	}
	modalities := normalizeModalities(embedding.Modalities)
	if len(modalities) == 0 {
		modalities = normalizeModalities(strings.FieldsFunc(firstMetadata(metadata, "embedding_modalities", "embeddingModalities", "modality"), func(r rune) bool {
			return r == ',' || r == ';'
		}))
	}
	return indexer.EmbeddingMetadata{
		Provider:    provider,
		Model:       model,
		Dimensions:  dimensions,
		ContentHash: contentHash,
		Version:     version,
		Modalities:  modalities,
	}
}

func normalizeProvenance(provenance indexer.Provenance, metadata map[string]string, sourceURI, sourceText string) indexer.Provenance {
	resolvedSourceURI := strings.TrimSpace(provenance.SourceURI)
	sourceHash := strings.TrimSpace(provenance.SourceHash)
	ingestedAt := strings.TrimSpace(provenance.IngestedAt)
	producedBy := strings.TrimSpace(provenance.ProducedBy)
	traceID := strings.TrimSpace(provenance.TraceID)
	if resolvedSourceURI == "" {
		resolvedSourceURI = firstNonEmpty(sourceURI, firstMetadata(metadata, "source_uri", "source", "path"))
	}
	if sourceHash == "" {
		sourceHash = firstMetadata(metadata, "source_hash", "content_hash")
	}
	if sourceHash == "" {
		sourceHash = indexer.SourceHashFromText(sourceText)
	}
	if traceID == "" {
		traceID = firstMetadata(metadata, "trace_id", "session_id")
	}
	return indexer.Provenance{
		SourceURI:  resolvedSourceURI,
		SourceHash: sourceHash,
		IngestedAt: ingestedAt,
		ProducedBy: producedBy,
		TraceID:    traceID,
		Metadata:   cloneMetadata(provenance.Metadata),
		Sources:    normalizeSourceReferences(provenance.Sources),
	}
}

func normalizeSourceMetadata(metadata map[string]string, document indexer.Document, provenance indexer.Provenance) map[string]string {
	out := cloneMetadata(metadata)
	copyMissingMetadata(out, document.Metadata)
	copyMissingMetadata(out, provenance.Metadata)
	setMetadataDefault(out, "document_id", document.ID)
	setMetadataDefault(out, "document_name", document.Name)
	setMetadataDefault(out, "document_type", document.Type)
	setMetadataDefault(out, "source_uri", firstNonEmpty(document.SourceURI, provenance.SourceURI))
	setMetadataDefault(out, "source_hash", provenance.SourceHash)
	if len(document.Sources) > 0 {
		setMetadataDefault(out, "modality", document.Sources[0].Modality)
		setMetadataDefault(out, "mime_type", document.Sources[0].MIMEType)
	}
	if firstMetadata(out, "filename") == "" {
		setMetadataDefault(out, "filename", bestSourceFilename(out, document, provenance))
	}
	return out
}

func copyMissingMetadata(dst, src map[string]string) {
	for key, value := range src {
		setMetadataDefault(dst, key, value)
	}
}

func setMetadataDefault(metadata map[string]string, key, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if strings.TrimSpace(metadata[key]) != "" {
		return
	}
	metadata[key] = value
}

func bestSourceFilename(metadata map[string]string, document indexer.Document, provenance indexer.Provenance) string {
	for _, value := range []string{
		firstMetadata(metadata, "filename", "source_filename", "file_name"),
		firstMetadata(document.Metadata, "filename", "source_filename", "file_name"),
		firstMetadata(provenance.Metadata, "filename", "source_filename", "file_name"),
		document.SourceURI,
		provenance.SourceURI,
		firstMetadata(metadata, "source_uri", "source", "path"),
		document.Name,
	} {
		filename := filenameFromSource(value)
		if filename == "" {
			continue
		}
		if strings.EqualFold(value, document.Name) && !looksLikeFilename(filename) {
			continue
		}
		return filename
	}
	return ""
}

func filenameFromSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, "://"); idx >= 0 {
		withoutScheme := value[idx+3:]
		if slash := strings.IndexAny(withoutScheme, `/\`); slash >= 0 {
			value = withoutScheme[slash:]
		}
	}
	if query := strings.IndexAny(value, "?#"); query >= 0 {
		value = value[:query]
	}
	value = strings.TrimRight(value, `/\`)
	if slash := strings.LastIndexAny(value, `/\`); slash >= 0 {
		value = value[slash+1:]
	}
	value = strings.TrimSpace(value)
	if value == "." || value == ".." {
		return ""
	}
	return value
}

func looksLikeFilename(value string) bool {
	value = filenameFromSource(value)
	return strings.Contains(value, ".")
}

func normalizeCitations(citations []indexer.Citation, chunkID, sourceURI string) []indexer.Citation {
	out := make([]indexer.Citation, 0, len(citations)+1)
	for _, citation := range citations {
		id := strings.TrimSpace(citation.ID)
		resolvedSourceURI := strings.TrimSpace(citation.SourceURI)
		resolvedChunkID := strings.TrimSpace(citation.ChunkID)
		textSpan := strings.TrimSpace(citation.TextSpan)
		confidence := citation.Confidence
		if resolvedChunkID == "" {
			resolvedChunkID = chunkID
		}
		if resolvedSourceURI == "" {
			resolvedSourceURI = sourceURI
		}
		if id == "" {
			id = indexer.EntityIDFromName(firstNonEmpty(resolvedSourceURI, resolvedChunkID, textSpan))
		}
		if confidence == 0 {
			confidence = 1
		}
		if resolvedSourceURI == "" && resolvedChunkID == "" && textSpan == "" {
			continue
		}
		out = append(out, indexer.Citation{
			ID:          id,
			SourceURI:   resolvedSourceURI,
			ChunkID:     resolvedChunkID,
			TextSpan:    textSpan,
			StartOffset: citation.StartOffset,
			EndOffset:   citation.EndOffset,
			Confidence:  confidence,
			PageNumber:  citation.PageNumber,
			MediaRef:    strings.TrimSpace(citation.MediaRef),
			Modality:    strings.TrimSpace(citation.Modality),
		})
	}
	if len(out) == 0 && sourceURI != "" {
		out = append(out, indexer.Citation{
			ID:         indexer.EntityIDFromName(sourceURI + "#" + chunkID),
			SourceURI:  sourceURI,
			ChunkID:    chunkID,
			Confidence: 1,
		})
	}
	return out
}

func normalizeFacts(facts []indexer.Fact, chunkID, sourceURI string) []indexer.Fact {
	out := make([]indexer.Fact, 0, len(facts))
	for _, fact := range facts {
		id := strings.TrimSpace(fact.ID)
		subject := strings.TrimSpace(fact.Subject)
		predicate := strings.TrimSpace(fact.Predicate)
		object := strings.TrimSpace(fact.Object)
		confidence := fact.Confidence
		if subject == "" || predicate == "" || object == "" {
			continue
		}
		if id == "" {
			id = indexer.EntityIDFromName(subject + "|" + predicate + "|" + object)
		}
		if confidence == 0 {
			confidence = 1
		}
		out = append(out, indexer.Fact{
			ID:         id,
			Subject:    subject,
			Predicate:  predicate,
			Object:     object,
			Confidence: confidence,
			Citations:  normalizeCitations(fact.Citations, chunkID, sourceURI),
			Metadata:   cloneMetadata(fact.Metadata),
		})
	}
	return out
}

func relationEndpoints(relation indexer.Relation) []indexer.Entity {
	if !completeRelation(relation) {
		return nil
	}
	return []indexer.Entity{
		{ID: relation.FromID, Name: relation.FromID, Type: unknownEntityType},
		{ID: relation.ToID, Name: relation.ToID, Type: unknownEntityType},
	}
}

func primaryEntities(cmd IndexCommand) []indexer.Entity {
	entities := make([]indexer.Entity, 0, len(cmd.Entities)+len(cmd.Relations)*2)
	seen := make(map[string]int)
	add := func(entity indexer.Entity) {
		entity = normalizeEntityForWrite(entity)
		if entity.ID == "" {
			return
		}
		if existing, ok := seen[entity.ID]; ok {
			if entities[existing].Type == unknownEntityType && entity.Type != unknownEntityType {
				entities[existing] = entity
			}
			return
		}
		seen[entity.ID] = len(entities)
		entities = append(entities, entity)
	}
	for _, entity := range cmd.Entities {
		add(entity)
	}
	for _, relation := range cmd.Relations {
		for _, endpoint := range relationEndpoints(relation) {
			add(endpoint)
		}
	}
	return entities
}

func uniqueRelations(relations []indexer.Relation) []indexer.Relation {
	out := make([]indexer.Relation, 0, len(relations))
	seen := make(map[string]struct{}, len(relations))
	for _, relation := range relations {
		if !completeRelation(relation) {
			continue
		}
		key := relation.FromID + "\x00" + relation.Relation + "\x00" + relation.ToID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, relation)
	}
	return out
}

func normalizeEntityForWrite(entity indexer.Entity) indexer.Entity {
	id := strings.TrimSpace(entity.ID)
	name := strings.TrimSpace(entity.Name)
	typ := strings.TrimSpace(entity.Type)
	if id == "" {
		id = indexer.EntityIDFromName(name)
	}
	if name == "" {
		name = id
	}
	if typ == "" {
		typ = unknownEntityType
	}
	return indexer.Entity{ID: id, Name: name, Type: typ}
}

func completeRelation(relation indexer.Relation) bool {
	return relation.FromID != "" && relation.ToID != "" && relation.Relation != ""
}

func firstMetadata(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parsePositiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func validConfidence(value float32) bool {
	return value >= 0 && value <= 1
}

func normalizeSourceReferences(in []indexer.SourceReference) []indexer.SourceReference {
	out := make([]indexer.SourceReference, 0, len(in))
	for _, source := range in {
		source = indexer.SourceReference{
			Modality:    strings.TrimSpace(source.Modality),
			MIMEType:    strings.TrimSpace(source.MIMEType),
			PageNumber:  source.PageNumber,
			ContentRef:  strings.TrimSpace(source.ContentRef),
			MediaRef:    strings.TrimSpace(source.MediaRef),
			ContentHash: strings.TrimSpace(source.ContentHash),
			SourceURI:   strings.TrimSpace(source.SourceURI),
			Metadata:    cloneMetadata(source.Metadata),
		}
		if source.Modality == "" && source.MIMEType == "" && source.PageNumber == 0 && source.ContentRef == "" && source.MediaRef == "" && source.ContentHash == "" && source.SourceURI == "" && len(source.Metadata) == 0 {
			continue
		}
		out = append(out, source)
	}
	return out
}

func normalizeModalities(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, modality := range in {
		modality = strings.ToLower(strings.TrimSpace(modality))
		if modality == "" {
			continue
		}
		if _, ok := seen[modality]; ok {
			continue
		}
		seen[modality] = struct{}{}
		out = append(out, modality)
	}
	return out
}
