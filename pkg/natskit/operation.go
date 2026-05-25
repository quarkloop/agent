package natskit

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	ServiceSubjectPrefix = "svc"
	DefaultVersion       = "v1"
)

var tokenPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Operation is a concrete NATS request/reply endpoint. Subject is the only
// routing identity transmitted on the wire; Owner and Function are registry
// metadata used for diagnostics and audit records.
type Operation struct {
	Owner    string
	Function string
	Subject  string
}

func ServiceOperation(owner, function string) (Operation, error) {
	ownerToken, err := subjectToken("owner", owner)
	if err != nil {
		return Operation{}, err
	}
	functionToken, err := subjectToken("function", function)
	if err != nil {
		return Operation{}, err
	}
	return Operation{
		Owner:    ownerToken,
		Function: functionToken,
		Subject:  fmt.Sprintf("%s.%s.%s.%s", ServiceSubjectPrefix, ownerToken, DefaultVersion, functionToken),
	}, nil
}

func ServiceOperationFromFunctionName(owner, functionName string) (Operation, error) {
	ownerToken, err := subjectToken("owner", owner)
	if err != nil {
		return Operation{}, err
	}
	function := strings.TrimSpace(functionName)
	if function == "" {
		return Operation{}, fmt.Errorf("function name is required")
	}
	for _, prefix := range []string{owner + "_", ownerToken + "_"} {
		if strings.HasPrefix(function, prefix) {
			function = strings.TrimPrefix(function, prefix)
			break
		}
	}
	return ServiceOperation(ownerToken, function)
}

func ParseServiceOperation(subject string) (Operation, error) {
	parts := strings.Split(strings.TrimSpace(subject), ".")
	if len(parts) != 4 || parts[0] != ServiceSubjectPrefix || parts[2] != DefaultVersion {
		return Operation{}, fmt.Errorf("service operation subject %q must match svc.<owner>.v1.<function>", subject)
	}
	for _, item := range []struct {
		name  string
		value string
	}{{"owner", parts[1]}, {"function", parts[3]}} {
		if !tokenPattern.MatchString(item.value) {
			return Operation{}, fmt.Errorf("service operation subject %q has invalid %s token %q", subject, item.name, item.value)
		}
	}
	return Operation{Owner: parts[1], Function: parts[3], Subject: strings.TrimSpace(subject)}, nil
}

func Subject(subject string) (string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" || strings.ContainsAny(subject, " \t\r\n*>") {
		return "", fmt.Errorf("concrete nats subject %q is invalid", subject)
	}
	return subject, nil
}

// SubscriptionSubject accepts NATS subscription wildcards. Request and
// publish APIs intentionally use Subject instead because publishers must
// address concrete destinations.
func SubscriptionSubject(subject string) (string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" || strings.ContainsAny(subject, " \t\r\n") {
		return "", fmt.Errorf("nats subscription subject %q is invalid", subject)
	}
	parts := strings.Split(subject, ".")
	for i, part := range parts {
		if part == "" {
			return "", fmt.Errorf("nats subscription subject %q contains an empty token", subject)
		}
		if strings.ContainsAny(part, "*>") && part != "*" && !(part == ">" && i == len(parts)-1) {
			return "", fmt.Errorf("nats subscription subject %q has an invalid wildcard token", subject)
		}
	}
	return subject, nil
}

func subjectToken(name, value string) (string, error) {
	token := stableToken(value)
	if token == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if !tokenPattern.MatchString(token) {
		return "", fmt.Errorf("%s token %q is invalid", name, token)
	}
	return token, nil
}

func stableToken(value string) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	lastUnderscore := false
	prevLowerOrDigit := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && prevLowerOrDigit && !lastUnderscore && out.Len() > 0 {
				out.WriteByte('_')
			}
			out.WriteRune(unicode.ToLower(r))
			lastUnderscore = false
			prevLowerOrDigit = unicode.IsLower(r) || unicode.IsDigit(r)
		case r == '_' || r == '-' || r == '.':
			if !lastUnderscore && out.Len() > 0 {
				out.WriteByte('_')
				lastUnderscore = true
			}
			prevLowerOrDigit = false
		}
	}
	token := strings.Trim(out.String(), "_")
	if token != "" && token[0] >= '0' && token[0] <= '9' {
		token = "v" + token
	}
	return token
}
