package gatewaysvc

import (
	"fmt"
	"strings"
)

func validateResolvedEmbeddingInputs(inputs []multimodalInput) error {
	if len(inputs) == 0 {
		return fmt.Errorf("inputs are required")
	}
	for i, input := range inputs {
		if len(input.Content) == 0 {
			return fmt.Errorf("inputs[%d].content is required", i)
		}
		for j, part := range input.Content {
			if err := validateResolvedContentPart(part); err != nil {
				return fmt.Errorf("inputs[%d].content[%d]: %w", i, j, err)
			}
		}
	}
	return nil
}

func validateResolvedContentPart(part contentPart) error {
	switch part.Kind {
	case contentText:
		if strings.TrimSpace(part.Text) == "" {
			return fmt.Errorf("text is required for text content")
		}
	case contentImageURL:
		if strings.TrimSpace(part.ImageURL) == "" {
			return fmt.Errorf("image_url is required for image URL content")
		}
	case contentImageData:
		if len(part.ImageData) == 0 {
			return fmt.Errorf("image_data is required for image data content")
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(part.MIMEType)), "image/") {
			return fmt.Errorf("image data requires an image MIME type")
		}
	case contentContentRef, contentImageRef, contentPageRef, contentArtifactRef, contentFileRef:
		return fmt.Errorf("runtime reference %q must be resolved before Gateway provider dispatch", strings.TrimSpace(part.Ref))
	default:
		return fmt.Errorf("content kind is required")
	}
	return nil
}

func isTextOnlyInput(input multimodalInput) bool {
	for _, part := range input.Content {
		if part.Kind != contentText {
			return false
		}
	}
	return true
}

func containsMediaInput(inputs []multimodalInput) bool {
	for _, input := range inputs {
		if !isTextOnlyInput(input) {
			return true
		}
	}
	return false
}

func textOnlyEmbeddingInputs(inputs []multimodalInput) ([]string, error) {
	out := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if !isTextOnlyInput(input) {
			return nil, fmt.Errorf("Bifrost embedding adapter does not support multimodal inputs")
		}
		var text strings.Builder
		for _, part := range input.Content {
			text.WriteString(part.Text)
		}
		out = append(out, text.String())
	}
	return out, nil
}

func embeddingTextForUsage(inputs []multimodalInput) []string {
	out := make([]string, 0, len(inputs))
	for _, input := range inputs {
		for _, part := range input.Content {
			if part.Kind == contentText {
				out = append(out, part.Text)
			}
		}
	}
	return out
}

func textMessageContent(parts []contentPart) (string, error) {
	var text strings.Builder
	for _, part := range parts {
		if part.Kind != contentText {
			return "", fmt.Errorf("Bifrost chat adapter does not support multimodal message content")
		}
		text.WriteString(part.Text)
	}
	return text.String(), nil
}
