package dgraph

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const neighborhoodQuery = `query neighborhood($id: string) {
  chunks(func: eq(quark.chunk_id, $id)) {
    quark.chunk_entity {
      quark.entity_id
      quark.entity_name
      quark.entity_type
    }
  }
  entities(func: eq(quark.entity_id, $id)) {
    quark.entity_id
    quark.entity_name
    quark.entity_type
    out: ~quark.relation_from {
      quark.relation_name
      quark.relation_to {
        quark.entity_id
        quark.entity_name
        quark.entity_type
      }
    }
    in: ~quark.relation_to {
      quark.relation_name
      quark.relation_from {
        quark.entity_id
        quark.entity_name
        quark.entity_type
      }
    }
  }
}`

const chunkMetadataQuery = `query chunkMetadata($id: string) {
  chunks(func: eq(quark.chunk_id, $id), first: 1) {
    quark.metadata_json
  }
}`

func vectorSearchQuery(limit int, filters map[string]string) string {
	return fmt.Sprintf(`query search($vec: float32vector) {
  var(func: similar_to(quark.embedding, %d, $vec)) %s {
    emb as quark.embedding
    score as Math((1.0 + (($vec) dot emb)) / 2.0)
  }
  chunks(func: uid(score), orderdesc: val(score)) {
    quark.chunk_id
    quark.text_content
    quark.metadata_json
    quark.canonical_json
    score: val(score)
  }
}`, limit, dgraphFilter(filters))
}

func metadataSearchQuery(limit int, filters map[string]string) string {
	return fmt.Sprintf(`query search {
  chunks(func: has(quark.chunk_id), first: %d) %s {
    quark.chunk_id
    quark.text_content
    quark.metadata_json
    quark.canonical_json
  }
}`, limit, dgraphFilter(filters))
}

var predicateSafe = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func metadataPredicate(key string) string {
	clean := predicateSafe.ReplaceAllString(key, "_")
	clean = strings.Trim(clean, "_")
	if clean == "" {
		clean = "field"
	}
	sum := sha1.Sum([]byte(key))
	return "quark.meta_" + clean + "_" + hex.EncodeToString(sum[:4])
}

func dgraphFilter(filters map[string]string) string {
	if len(filters) == 0 {
		return ""
	}
	parts := make([]string, 0, len(filters))
	for key, value := range filters {
		parts = append(parts, fmt.Sprintf("eq(%s, %s)", metadataPredicate(key), quote(value)))
	}
	sort.Strings(parts)
	return "@filter(" + strings.Join(parts, " AND ") + ")"
}

func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, value := range v {
		parts[i] = strconv.FormatFloat(float64(value), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func quote(s string) string {
	return strconv.Quote(s)
}
