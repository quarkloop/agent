package runstatesvc

type runStatus string

const (
	statusPending   runStatus = "pending"
	statusRunning   runStatus = "running"
	statusSucceeded runStatus = "succeeded"
	statusFailed    runStatus = "failed"
	statusSkipped   runStatus = "skipped"
	statusCancelled runStatus = "cancelled"
)

type runRecord struct {
	ID                 string            `json:"id"`
	Space              string            `json:"space"`
	Title              string            `json:"title"`
	Kind               string            `json:"kind"`
	ActorRef           string            `json:"actor_ref,omitempty"`
	Status             runStatus         `json:"status"`
	Items              []itemRecord      `json:"items"`
	References         []referenceRecord `json:"references,omitempty"`
	ServiceCallRefs    []string          `json:"service_call_refs,omitempty"`
	CreatedAt          string            `json:"created_at"`
	UpdatedAt          string            `json:"updated_at"`
	RetentionExpiresAt string            `json:"retention_expires_at,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type itemRecord struct {
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	ResourceURI     string            `json:"resource_uri,omitempty"`
	Name            string            `json:"name,omitempty"`
	ContentHash     string            `json:"content_hash,omitempty"`
	Phase           string            `json:"phase"`
	Status          runStatus         `json:"status"`
	LastError       string            `json:"last_error,omitempty"`
	Artifacts       []artifactRecord  `json:"artifacts,omitempty"`
	Phases          []phaseRecord     `json:"phases,omitempty"`
	ServiceCallRefs []string          `json:"service_call_refs,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	RetryCount      int32             `json:"retry_count,omitempty"`
}

type artifactRecord struct {
	Ref       string            `json:"ref"`
	Kind      string            `json:"kind"`
	ItemID    string            `json:"item_id,omitempty"`
	CreatedAt string            `json:"created_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type referenceRecord struct {
	Ref       string            `json:"ref"`
	Kind      string            `json:"kind"`
	ItemID    string            `json:"item_id,omitempty"`
	CreatedAt string            `json:"created_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type phaseRecord struct {
	Phase           string            `json:"phase"`
	Status          runStatus         `json:"status"`
	ArtifactRef     string            `json:"artifact_ref,omitempty"`
	Error           string            `json:"error,omitempty"`
	UpdatedAt       string            `json:"updated_at"`
	ServiceCallRefs []string          `json:"service_call_refs,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type leaseRecord struct {
	Key       string `json:"key"`
	OwnerID   string `json:"owner_id"`
	RunID     string `json:"run_id"`
	ItemID    string `json:"item_id,omitempty"`
	ExpiresAt string `json:"expires_at"`
	Revision  uint64 `json:"revision"`
}
