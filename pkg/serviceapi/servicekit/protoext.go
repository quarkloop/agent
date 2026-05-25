package servicekit

import servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"

// CloneDescriptor returns a deep-ish copy of the descriptor. Generated proto
// Clone is intentionally avoided here to keep callers working with concrete
// types.
func CloneDescriptor(x *servicev1.ServiceDescriptor) *servicev1.ServiceDescriptor {
	if x == nil {
		return nil
	}
	out := &servicev1.ServiceDescriptor{
		Name:    x.GetName(),
		Type:    x.GetType(),
		Version: x.GetVersion(),
		Address: x.GetAddress(),
		Rpcs:    make([]*servicev1.RpcDescriptor, 0, len(x.GetRpcs())),
		Skills:  make([]*servicev1.SkillDescriptor, 0, len(x.GetSkills())),
	}
	for _, r := range x.GetRpcs() {
		out.Rpcs = append(out.Rpcs, &servicev1.RpcDescriptor{
			Service:              r.GetService(),
			Method:               r.GetMethod(),
			Request:              r.GetRequest(),
			Response:             r.GetResponse(),
			Description:          r.GetDescription(),
			Owner:                r.GetOwner(),
			FunctionName:         r.GetFunctionName(),
			RiskLevel:            r.GetRiskLevel(),
			ApprovalRequired:     r.GetApprovalRequired(),
			ApprovalRequirements: append([]string(nil), r.GetApprovalRequirements()...),
			Streaming:            r.GetStreaming(),
			Idempotent:           r.GetIdempotent(),
			TimeoutMillis:        r.GetTimeoutMillis(),
			RetryPolicy:          cloneRetryPolicy(r.GetRetryPolicy()),
			Examples:             cloneExamples(r.GetExamples()),
			Subject:              r.GetSubject(),
		})
	}
	for _, s := range x.GetSkills() {
		out.Skills = append(out.Skills, &servicev1.SkillDescriptor{
			Name:     s.GetName(),
			Version:  s.GetVersion(),
			Markdown: s.GetMarkdown(),
		})
	}
	return out
}

func cloneRetryPolicy(x *servicev1.RetryPolicy) *servicev1.RetryPolicy {
	if x == nil {
		return nil
	}
	return &servicev1.RetryPolicy{
		MaxAttempts:          x.GetMaxAttempts(),
		RetryableCodes:       append([]string(nil), x.GetRetryableCodes()...),
		InitialBackoffMillis: x.GetInitialBackoffMillis(),
		MaxBackoffMillis:     x.GetMaxBackoffMillis(),
	}
}

func cloneExamples(examples []*servicev1.ServiceFunctionExample) []*servicev1.ServiceFunctionExample {
	out := make([]*servicev1.ServiceFunctionExample, 0, len(examples))
	for _, example := range examples {
		if example == nil {
			continue
		}
		out = append(out, &servicev1.ServiceFunctionExample{
			Name:        example.GetName(),
			Description: example.GetDescription(),
			RequestJson: example.GetRequestJson(),
		})
	}
	return out
}
