package boundary

// Diagnostic is the stable user-facing shape for explaining failures across
// provider, runtime, service, supervisor, CLI, and E2E boundaries.
type Diagnostic struct {
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	Boundary  string `json:"boundary"`
	Category  string `json:"category"`
	Operation string `json:"operation,omitempty"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
}

func DiagnosticFromError(err error, defaultBoundary Boundary, operation string) Diagnostic {
	boundaryErr := FromError(defaultBoundary, operation, err)
	code := string(boundaryErr.Boundary) + "." + string(boundaryErr.Category)
	return Diagnostic{
		Code:      code,
		Severity:  diagnosticSeverity(boundaryErr.Category),
		Boundary:  string(boundaryErr.Boundary),
		Category:  string(boundaryErr.Category),
		Operation: boundaryErr.Operation,
		Message:   boundaryErr.Message,
		Hint:      diagnosticHint(boundaryErr),
	}
}

func diagnosticSeverity(category Category) string {
	switch category {
	case Auth, RateLimit, PolicyDenied, ApprovalRequired:
		return "action_required"
	case InvalidArgument, NotFound, Conflict:
		return "user_fixable"
	case Unavailable, Transport, Deadline, ContextOverflow:
		return "retryable"
	default:
		return "error"
	}
}

func diagnosticHint(err *Error) string {
	if err == nil {
		return ""
	}
	switch err.Category {
	case Auth:
		return "Check the configured provider or service credentials for this space."
	case RateLimit:
		return "The provider reported quota or rate limiting; retry later or switch the configured model/provider."
	case PolicyDenied:
		return "Review the active agent profile and space configuration permission narrowing for this tool or service function."
	case ApprovalRequired:
		return "Approve the requested operation or ask the agent for a read-only plan."
	case NotFound:
		return "Check that the referenced service, tool, session, artifact, or runtime reference exists."
	case InvalidArgument:
		return "Check the request shape and retry with valid structured input."
	case Unavailable:
		if err.Boundary == Service {
			return "Inspect service status, readiness, descriptor validity, and backend dependencies."
		}
		return "Retry after the dependency is healthy."
	case Transport:
		return "Check endpoint connectivity and process health for the dependency."
	case Deadline:
		return "The operation timed out; inspect service logs and retry with a smaller request."
	case ContextOverflow:
		return "Reduce prompt, context, or extracted content size before retrying."
	default:
		return "Inspect the correlated runtime, service, and supervisor events."
	}
}
