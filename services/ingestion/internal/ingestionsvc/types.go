package ingestionsvc

type phase string

const (
	phaseRegistered phase = "registered"
	phaseParsed     phase = "parsed"
	phaseStructured phase = "structured"
	phaseEmbedded   phase = "embedded"
	phaseIndexed    phase = "indexed"
	phaseCited      phase = "cited"
)

type sourceStatus string

const (
	statusPending   sourceStatus = "pending"
	statusRunning   sourceStatus = "running"
	statusSucceeded sourceStatus = "succeeded"
	statusFailed    sourceStatus = "failed"
	statusSkipped   sourceStatus = "skipped"
	statusCancelled sourceStatus = "cancelled"
)

type runRecord struct {
	ID        string            `json:"id"`
	Space     string            `json:"space"`
	Title     string            `json:"title"`
	Status    sourceStatus      `json:"status"`
	Sources   []sourceRecord    `json:"sources"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type sourceRecord struct {
	ID          string            `json:"id"`
	SourceURI   string            `json:"source_uri,omitempty"`
	Filename    string            `json:"filename,omitempty"`
	SourceHash  string            `json:"source_hash,omitempty"`
	Phase       phase             `json:"phase"`
	Status      sourceStatus      `json:"status"`
	LastError   string            `json:"last_error,omitempty"`
	Artifacts   []artifactRecord  `json:"artifacts,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	FilePath    string            `json:"file_path,omitempty"`
	Extraction  stepRecord        `json:"extraction"`
	Structuring stepRecord        `json:"structuring"`
	Embedding   stepRecord        `json:"embedding"`
	Indexing    stepRecord        `json:"indexing"`
	Citation    stepRecord        `json:"citation"`
	RetryCount  int32             `json:"retry_count,omitempty"`
}

type artifactRecord struct {
	Ref       string            `json:"ref"`
	Kind      string            `json:"kind"`
	SourceID  string            `json:"source_id,omitempty"`
	CreatedAt string            `json:"created_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type stepRecord struct {
	Phase       phase             `json:"phase"`
	Status      sourceStatus      `json:"status"`
	ArtifactRef string            `json:"artifact_ref,omitempty"`
	Error       string            `json:"error,omitempty"`
	UpdatedAt   string            `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
