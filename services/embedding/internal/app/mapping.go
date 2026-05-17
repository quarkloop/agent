package app

import (
	"strings"

	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
)

type embedCommand struct {
	Input      string
	Model      string
	Dimensions int
}

func embedCommandFromProto(req *embeddingv1.EmbedRequest) embedCommand {
	if req == nil {
		return embedCommand{}
	}
	return embedCommand{
		Input:      req.GetInput(),
		Model:      strings.TrimSpace(req.GetModel()),
		Dimensions: int(req.GetDimensions()),
	}
}

func embedResponseToProto(result embeddingResult, contentHash string) *embeddingv1.EmbedResponse {
	return &embeddingv1.EmbedResponse{
		Vector:      append([]float32(nil), result.Vector...),
		Model:       result.Model,
		Dimensions:  int32(len(result.Vector)),
		Provider:    result.Provider,
		ContentHash: contentHash,
	}
}
