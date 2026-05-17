package ingestionsvc

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

func cloneRun(in runRecord) runRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	out.Sources = make([]sourceRecord, len(in.Sources))
	for i, source := range in.Sources {
		out.Sources[i] = cloneSource(source)
	}
	return out
}

func cloneSource(in sourceRecord) sourceRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	out.Artifacts = make([]artifactRecord, len(in.Artifacts))
	for i, artifact := range in.Artifacts {
		out.Artifacts[i] = cloneArtifact(artifact)
	}
	out.Extraction = cloneStep(in.Extraction)
	out.Structuring = cloneStep(in.Structuring)
	out.Embedding = cloneStep(in.Embedding)
	out.Indexing = cloneStep(in.Indexing)
	out.Citation = cloneStep(in.Citation)
	return out
}

func cloneArtifact(in artifactRecord) artifactRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	return out
}

func cloneStep(in stepRecord) stepRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	return out
}
