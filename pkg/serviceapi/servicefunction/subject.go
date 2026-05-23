package servicefunction

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	SubjectPrefix  = "svc"
	DefaultVersion = "v1"
)

var subjectTokenPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func Subject(service, version, function string) (string, error) {
	serviceToken, err := normalizeSubjectToken("service", service)
	if err != nil {
		return "", err
	}
	versionToken, err := normalizeSubjectToken("version", version)
	if err != nil {
		return "", err
	}
	functionToken, err := normalizeSubjectToken("function", function)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s.%s.%s", SubjectPrefix, serviceToken, versionToken, functionToken), nil
}

func SubjectFromFunctionName(functionName string) (string, error) {
	owner, function, ok := strings.Cut(strings.TrimSpace(functionName), "_")
	if !ok {
		return "", fmt.Errorf("service function name %q must be owner_function", functionName)
	}
	return Subject(owner, DefaultVersion, function)
}

func SubjectFromOwnerAndFunctionName(owner, functionName string) (string, error) {
	ownerToken, err := normalizeSubjectToken("owner", owner)
	if err != nil {
		return "", err
	}
	function := strings.TrimSpace(functionName)
	if function == "" {
		return "", fmt.Errorf("service function name is required")
	}
	for _, prefix := range []string{owner + "_", ownerToken + "_"} {
		if strings.HasPrefix(function, prefix) {
			function = strings.TrimPrefix(function, prefix)
			break
		}
	}
	return Subject(ownerToken, DefaultVersion, function)
}

func FunctionTokenFromOwnerAndFunctionName(owner, functionName string) (string, error) {
	subject, err := SubjectFromOwnerAndFunctionName(owner, functionName)
	if err != nil {
		return "", err
	}
	parts := strings.Split(subject, ".")
	if len(parts) != 4 {
		return "", fmt.Errorf("service function subject %q is invalid", subject)
	}
	return parts[3], nil
}

func ValidateSubject(subject string) error {
	parts := strings.Split(strings.TrimSpace(subject), ".")
	if len(parts) != 4 || parts[0] != SubjectPrefix {
		return fmt.Errorf("service function subject %q must match svc.<service>.<version>.<function>", subject)
	}
	for i, part := range parts[1:] {
		name := []string{"service", "version", "function"}[i]
		if !subjectTokenPattern.MatchString(part) {
			return fmt.Errorf("service function subject %q has invalid %s token %q", subject, name, part)
		}
	}
	return nil
}

func normalizeSubjectToken(name, value string) (string, error) {
	token := stableToken(value)
	if token == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if !subjectTokenPattern.MatchString(token) {
		return "", fmt.Errorf("%s token %q is invalid", name, token)
	}
	return token, nil
}

func stableToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	prevLowerOrDigit := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && prevLowerOrDigit && !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			lastUnderscore = false
			prevLowerOrDigit = unicode.IsLower(r) || unicode.IsDigit(r)
		case r == '_' || r == '-' || r == '.':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
			prevLowerOrDigit = false
		}
	}
	token := strings.Trim(b.String(), "_")
	if token == "" {
		return ""
	}
	if token[0] >= '0' && token[0] <= '9' {
		token = "v" + token
	}
	return token
}

func requireNonEmpty(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New(name + " is required")
	}
	return nil
}
