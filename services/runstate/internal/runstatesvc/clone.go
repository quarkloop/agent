package runstatesvc

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

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return append([]string(nil), in...)
}

func cloneRun(in runRecord) runRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	out.ServiceCallRefs = cloneStrings(in.ServiceCallRefs)
	out.Items = make([]itemRecord, len(in.Items))
	for i, item := range in.Items {
		out.Items[i] = cloneItem(item)
	}
	out.References = make([]referenceRecord, len(in.References))
	for i, ref := range in.References {
		out.References[i] = cloneReference(ref)
	}
	return out
}

func cloneItem(in itemRecord) itemRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	out.ServiceCallRefs = cloneStrings(in.ServiceCallRefs)
	out.Artifacts = make([]artifactRecord, len(in.Artifacts))
	for i, artifact := range in.Artifacts {
		out.Artifacts[i] = cloneArtifact(artifact)
	}
	out.Phases = make([]phaseRecord, len(in.Phases))
	for i, phase := range in.Phases {
		out.Phases[i] = clonePhase(phase)
	}
	return out
}

func cloneArtifact(in artifactRecord) artifactRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	return out
}

func cloneReference(in referenceRecord) referenceRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	return out
}

func clonePhase(in phaseRecord) phaseRecord {
	out := in
	out.Metadata = cloneMap(in.Metadata)
	out.ServiceCallRefs = cloneStrings(in.ServiceCallRefs)
	return out
}
