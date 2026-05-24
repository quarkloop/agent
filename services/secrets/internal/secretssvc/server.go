package secretssvc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/boundary/redaction"
	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

type Backend interface {
	Resolve(context.Context, SecretRef, bool) (*secretsv1.SecretMaterial, error)
	IssueToken(context.Context, *secretsv1.IssueScopedSecretRequest) (*secretsv1.ScopedSecret, error)
	RenewLease(context.Context, string, int64) (*secretsv1.Lease, error)
	RevokeLease(context.Context, string, bool) error
	Rotate(context.Context, SecretRef, string, int64) (int64, error)
}

type Server struct {
	backend Backend
	audit   *AuditLog
	logger  *slog.Logger
}

func NewServer(backend Backend, logger *slog.Logger) (*Server, error) {
	if backend == nil {
		return nil, fmt.Errorf("secrets backend is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{backend: backend, audit: NewAuditLog(), logger: logger}, nil
}

func (s *Server) ResolveRef(ctx context.Context, req *secretsv1.ResolveRefRequest) (*secretsv1.ResolveRefResponse, error) {
	if req == nil || strings.TrimSpace(req.GetSecretRef()) == "" {
		return nil, serviceerrors.InvalidArgument("secret_ref is required")
	}
	ref, err := ParseSecretRef(req.GetSecretRef(), req.GetField())
	if err != nil {
		return nil, serviceerrors.InvalidArgument(err.Error())
	}
	secret, err := s.backend.Resolve(ctx, ref, req.GetIncludeValue())
	auditID := s.audit.Record(AuditEvent{
		SecretRef: ref.String(),
		Action:    "resolve_ref",
		ActorID:   req.GetActorId(),
		Outcome:   outcome(err),
		Metadata: map[string]string{
			"purpose": req.GetPurpose(),
			"field":   ref.Field,
		},
	})
	if err != nil {
		return nil, err
	}
	return &secretsv1.ResolveRefResponse{Secret: secret, AuditId: auditID}, nil
}

func (s *Server) IssueScopedSecret(ctx context.Context, req *secretsv1.IssueScopedSecretRequest) (*secretsv1.IssueScopedSecretResponse, error) {
	if req == nil || strings.TrimSpace(req.GetScope()) == "" {
		return nil, serviceerrors.InvalidArgument("scope is required")
	}
	secret, err := s.backend.IssueToken(ctx, req)
	auditID := s.audit.Record(AuditEvent{Action: "issue_scoped_secret", ActorID: req.GetScope(), Outcome: outcome(err), Metadata: cloneStringMap(req.GetMetadata())})
	if err != nil {
		return nil, err
	}
	return &secretsv1.IssueScopedSecretResponse{Secret: secret, AuditId: auditID}, nil
}

func (s *Server) RenewLease(ctx context.Context, req *secretsv1.RenewLeaseRequest) (*secretsv1.RenewLeaseResponse, error) {
	if req == nil || strings.TrimSpace(req.GetLeaseId()) == "" {
		return nil, serviceerrors.InvalidArgument("lease_id is required")
	}
	lease, err := s.backend.RenewLease(ctx, req.GetLeaseId(), req.GetIncrementSeconds())
	auditID := s.audit.Record(AuditEvent{SecretRef: req.GetLeaseId(), Action: "renew_lease", Outcome: outcome(err)})
	if err != nil {
		return nil, err
	}
	return &secretsv1.RenewLeaseResponse{Lease: lease, AuditId: auditID}, nil
}

func (s *Server) RevokeLease(ctx context.Context, req *secretsv1.RevokeLeaseRequest) (*secretsv1.RevokeLeaseResponse, error) {
	if req == nil || strings.TrimSpace(req.GetLeaseId()) == "" {
		return nil, serviceerrors.InvalidArgument("lease_id is required")
	}
	err := s.backend.RevokeLease(ctx, req.GetLeaseId(), req.GetSync())
	auditID := s.audit.Record(AuditEvent{SecretRef: req.GetLeaseId(), Action: "revoke_lease", Outcome: outcome(err), Metadata: map[string]string{"reason": req.GetReason()}})
	if err != nil {
		return nil, err
	}
	return &secretsv1.RevokeLeaseResponse{LeaseId: req.GetLeaseId(), Revoked: true, AuditId: auditID}, nil
}

func (s *Server) RotateSecret(ctx context.Context, req *secretsv1.RotateSecretRequest) (*secretsv1.RotateSecretResponse, error) {
	if req == nil || strings.TrimSpace(req.GetSecretRef()) == "" {
		return nil, serviceerrors.InvalidArgument("secret_ref is required")
	}
	if req.GetValue() == "" {
		return nil, serviceerrors.InvalidArgument("value is required")
	}
	ref, err := ParseSecretRef(req.GetSecretRef(), req.GetField())
	if err != nil {
		return nil, serviceerrors.InvalidArgument(err.Error())
	}
	version, err := s.backend.Rotate(ctx, ref, req.GetValue(), req.GetCheckAndSetVersion())
	auditID := s.audit.Record(AuditEvent{SecretRef: ref.String(), Action: "rotate_secret", Outcome: outcome(err), Metadata: map[string]string{"reason": req.GetReason()}})
	if err != nil {
		return nil, err
	}
	return &secretsv1.RotateSecretResponse{SecretRef: ref.String(), Version: version, AuditId: auditID}, nil
}

func (s *Server) AuditAccess(_ context.Context, req *secretsv1.AuditAccessRequest) (*secretsv1.AuditAccessResponse, error) {
	if req == nil || strings.TrimSpace(req.GetAction()) == "" {
		return nil, serviceerrors.InvalidArgument("action is required")
	}
	event := AuditEvent{
		SecretRef: req.GetSecretRef(),
		Action:    req.GetAction(),
		ActorID:   req.GetActorId(),
		Outcome:   req.GetOutcome(),
		Metadata:  cloneStringMap(req.GetMetadata()),
	}
	auditID := s.audit.Record(event)
	return &secretsv1.AuditAccessResponse{AuditId: auditID, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

type SecretRef struct {
	Mount string
	Path  string
	Field string
}

func ParseSecretRef(raw, field string) (SecretRef, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "bao://")
	value = strings.TrimPrefix(value, "openbao://")
	refPart, refField, ok := strings.Cut(value, "#")
	if ok && field == "" {
		field = refField
	}
	field = strings.TrimSpace(field)
	if field == "" {
		field = "value"
	}
	parts := strings.SplitN(strings.Trim(refPart, "/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return SecretRef{}, fmt.Errorf("secret_ref must be bao://<mount>/<path>#<field>")
	}
	return SecretRef{Mount: parts[0], Path: parts[1], Field: field}, nil
}

func (r SecretRef) String() string {
	return "bao://" + strings.Trim(r.Mount, "/") + "/" + strings.Trim(r.Path, "/") + "#" + r.Field
}

type AuditEvent struct {
	ID        string
	SecretRef string
	Action    string
	ActorID   string
	Outcome   string
	Metadata  map[string]string
	CreatedAt time.Time
}

type AuditLog struct {
	mu     sync.Mutex
	next   uint64
	events []AuditEvent
}

func NewAuditLog() *AuditLog {
	return &AuditLog{}
}

func (l *AuditLog) Record(event AuditEvent) string {
	if l == nil {
		return ""
	}
	now := time.Now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next++
	event.ID = fmt.Sprintf("secret-audit-%d", l.next)
	event.CreatedAt = now
	event.SecretRef = redaction.RedactString(event.SecretRef)
	event.ActorID = redaction.RedactString(event.ActorID)
	event.Metadata = redactMap(event.Metadata)
	l.events = append(l.events, event)
	return event.ID
}

func outcome(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func redactMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = redaction.RedactString(value)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
