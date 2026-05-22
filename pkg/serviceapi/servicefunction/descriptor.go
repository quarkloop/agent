package servicefunction

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const DescriptorVersion = 1

type RiskLevel string

const (
	RiskRead  RiskLevel = "read"
	RiskWrite RiskLevel = "write"
	RiskAdmin RiskLevel = "admin"
)

type ApprovalPolicy struct {
	Required     bool     `json:"required"`
	Requirements []string `json:"requirements,omitempty"`
}

type PermissionSet struct {
	PublishAllow   []string `json:"publish_allow,omitempty"`
	SubscribeAllow []string `json:"subscribe_allow,omitempty"`
}

type Example struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Request     json.RawMessage `json:"request"`
}

type Descriptor struct {
	Version       int             `json:"version"`
	Service       string          `json:"service"`
	Function      string          `json:"function"`
	Subject       string          `json:"subject"`
	InputSchema   json.RawMessage `json:"input_schema"`
	OutputSchema  json.RawMessage `json:"output_schema"`
	Risk          RiskLevel       `json:"risk"`
	Approval      ApprovalPolicy  `json:"approval"`
	Idempotent    bool            `json:"idempotent"`
	TimeoutMillis int64           `json:"timeout_millis"`
	Streaming     bool            `json:"streaming"`
	Examples      []Example       `json:"examples,omitempty"`
	Permissions   PermissionSet   `json:"permissions"`
}

func NewDescriptor(service, function string, opts DescriptorOptions) (Descriptor, error) {
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = DefaultVersion
	}
	subject, err := Subject(service, version, function)
	if err != nil {
		return Descriptor{}, err
	}
	descriptor := Descriptor{
		Version:       DescriptorVersion,
		Service:       stableToken(service),
		Function:      stableToken(function),
		Subject:       subject,
		InputSchema:   cloneRawMessage(opts.InputSchema),
		OutputSchema:  cloneRawMessage(opts.OutputSchema),
		Risk:          opts.Risk,
		Approval:      cloneApprovalPolicy(opts.Approval),
		Idempotent:    opts.Idempotent,
		TimeoutMillis: int64(opts.Timeout / time.Millisecond),
		Streaming:     opts.Streaming,
		Examples:      cloneExamples(opts.Examples),
		Permissions:   clonePermissionSet(opts.Permissions),
	}
	if descriptor.Risk == "" {
		descriptor.Risk = RiskRead
	}
	if descriptor.TimeoutMillis == 0 {
		descriptor.TimeoutMillis = int64(30 * time.Second / time.Millisecond)
	}
	if err := descriptor.Validate(); err != nil {
		return Descriptor{}, err
	}
	return descriptor, nil
}

type DescriptorOptions struct {
	Version      string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	Risk         RiskLevel
	Approval     ApprovalPolicy
	Idempotent   bool
	Timeout      time.Duration
	Streaming    bool
	Examples     []Example
	Permissions  PermissionSet
}

func (d Descriptor) Validate() error {
	if d.Version != DescriptorVersion {
		return fmt.Errorf("unsupported service function descriptor version %d", d.Version)
	}
	if err := requireNonEmpty("service", d.Service); err != nil {
		return err
	}
	if err := requireNonEmpty("function", d.Function); err != nil {
		return err
	}
	if err := ValidateSubject(d.Subject); err != nil {
		return err
	}
	expectedSubject, err := Subject(d.Service, subjectVersion(d.Subject), d.Function)
	if err != nil {
		return err
	}
	if d.Subject != expectedSubject {
		return fmt.Errorf("subject %q does not match service/function %q/%q", d.Subject, d.Service, d.Function)
	}
	if err := validateSchema("input_schema", d.InputSchema); err != nil {
		return err
	}
	if err := validateSchema("output_schema", d.OutputSchema); err != nil {
		return err
	}
	switch d.Risk {
	case RiskRead, RiskWrite, RiskAdmin:
	default:
		return fmt.Errorf("invalid risk level %q", d.Risk)
	}
	if d.TimeoutMillis <= 0 {
		return fmt.Errorf("timeout_millis must be positive")
	}
	for i, example := range d.Examples {
		if strings.TrimSpace(example.Name) == "" {
			return fmt.Errorf("examples[%d].name is required", i)
		}
		if !json.Valid(example.Request) {
			return fmt.Errorf("examples[%d].request must be valid JSON", i)
		}
	}
	return nil
}

func (d Descriptor) Timeout(defaultTimeout time.Duration) time.Duration {
	if d.TimeoutMillis <= 0 {
		return defaultTimeout
	}
	return time.Duration(d.TimeoutMillis) * time.Millisecond
}

func (d Descriptor) Clone() Descriptor {
	out := d
	out.InputSchema = cloneRawMessage(d.InputSchema)
	out.OutputSchema = cloneRawMessage(d.OutputSchema)
	out.Approval = cloneApprovalPolicy(d.Approval)
	out.Examples = cloneExamples(d.Examples)
	out.Permissions = clonePermissionSet(d.Permissions)
	return out
}

func subjectVersion(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) != 4 {
		return ""
	}
	return parts[2]
}

func validateSchema(name string, schema json.RawMessage) error {
	if len(schema) == 0 {
		return fmt.Errorf("%s is required", name)
	}
	if !json.Valid(schema) {
		return fmt.Errorf("%s must be valid JSON", name)
	}
	return nil
}

func cloneApprovalPolicy(in ApprovalPolicy) ApprovalPolicy {
	return ApprovalPolicy{
		Required:     in.Required,
		Requirements: append([]string(nil), in.Requirements...),
	}
}

func clonePermissionSet(in PermissionSet) PermissionSet {
	return PermissionSet{
		PublishAllow:   append([]string(nil), in.PublishAllow...),
		SubscribeAllow: append([]string(nil), in.SubscribeAllow...),
	}
}

func cloneExamples(in []Example) []Example {
	out := make([]Example, 0, len(in))
	for _, example := range in {
		cp := example
		cp.Request = cloneRawMessage(example.Request)
		out = append(out, cp)
	}
	return out
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), in...)
}
