package docsvc

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
)

func inputFromProto(input *documentv1.DocumentInput) documentInput {
	if input == nil {
		return documentInput{}
	}
	return documentInput{
		SourceURI: strings.TrimSpace(input.GetSourceUri()),
		Content:   cloneBytes(input.GetContent()),
		Filename:  strings.TrimSpace(input.GetFilename()),
		MIMEType:  strings.TrimSpace(input.GetMimeType()),
		Metadata:  cloneMap(input.GetMetadata()),
	}
}

func resolveSource(_ context.Context, input documentInput) (sourceDocument, error) {
	source := sourceDocument{
		SourceURI: input.SourceURI,
		Content:   cloneBytes(input.Content),
		Filename:  input.Filename,
		MIMEType:  input.MIMEType,
		Metadata:  cloneMap(input.Metadata),
	}
	if len(source.Content) > 0 {
		if source.Filename == "" {
			source.Filename = filenameFromURI(source.SourceURI)
		}
		return source, nil
	}
	if input.SourceURI != "" {
		path, err := pathFromSourceURI(input.SourceURI)
		if err != nil {
			return sourceDocument{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return sourceDocument{}, fmt.Errorf("read document source %s: %w", path, err)
		}
		source.Path = path
		source.Content = data
		if source.Filename == "" {
			source.Filename = filepath.Base(path)
		}
		return source, nil
	}
	return sourceDocument{}, errEmptyInput
}

func pathFromSourceURI(sourceURI string) (string, error) {
	if strings.HasPrefix(sourceURI, "file://") {
		parsed, err := url.Parse(sourceURI)
		if err != nil {
			return "", fmt.Errorf("parse file source_uri: %w", err)
		}
		if parsed.Host != "" && parsed.Host != "localhost" {
			return "", fmt.Errorf("unsupported file source host %q", parsed.Host)
		}
		if parsed.Path == "" {
			return "", errEmptyInput
		}
		return filepath.Clean(parsed.Path), nil
	}
	if strings.Contains(sourceURI, "://") {
		return "", fmt.Errorf("unsupported source_uri scheme in %q", sourceURI)
	}
	return filepath.Clean(sourceURI), nil
}

func filenameFromURI(sourceURI string) string {
	if sourceURI == "" {
		return ""
	}
	path, err := pathFromSourceURI(sourceURI)
	if err != nil {
		return ""
	}
	return filepath.Base(path)
}

func cloneBytes(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
