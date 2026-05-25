package gatewaysvc

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

func multimodalInputHash(input multimodalInput) string {
	type hashPart struct {
		Kind      contentKind       `json:"kind"`
		Text      string            `json:"text,omitempty"`
		ImageURL  string            `json:"image_url,omitempty"`
		ImageData []byte            `json:"image_data,omitempty"`
		MIMEType  string            `json:"mime_type,omitempty"`
		Metadata  map[string]string `json:"metadata,omitempty"`
	}
	parts := make([]hashPart, 0, len(input.Content))
	for _, part := range input.Content {
		parts = append(parts, hashPart{
			Kind:      part.Kind,
			Text:      part.Text,
			ImageURL:  part.ImageURL,
			ImageData: append([]byte(nil), part.ImageData...),
			MIMEType:  part.MIMEType,
			Metadata:  cloneStringMap(part.Metadata),
		})
	}
	data, _ := json.Marshal(struct {
		Content  []hashPart        `json:"content"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}{Content: parts, Metadata: cloneStringMap(input.Metadata)})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
