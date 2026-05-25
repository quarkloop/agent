package clientcontract

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const Version = "v1"

var tokenPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

const (
	SubjectSpaceCreate       = "control.space.v1.create"
	SubjectSpaceList         = "control.space.v1.list"
	SubjectSpaceGet          = "control.space.v1.get"
	SubjectSpaceUpdate       = "control.space.v1.update"
	SubjectSpaceDelete       = "control.space.v1.delete"
	SubjectSpaceQuarkfile    = "control.space.v1.quarkfile"
	SubjectSpaceDoctor       = "control.space.v1.doctor"
	SubjectSpaceCredential   = "control.space.v1.credential"
	SubjectRuntimeCredential = "control.space.v1.runtime_credential"
	SubjectSessionCreate     = "control.session.v1.create"
	SubjectSessionList       = "control.session.v1.list"
	SubjectSessionGet        = "control.session.v1.get"
	SubjectSessionDelete     = "control.session.v1.delete"
	SubjectSessionCredential = "control.session.v1.credential"
	SubjectKBGet             = "control.kb.v1.get"
	SubjectKBSet             = "control.kb.v1.set"
	SubjectKBDelete          = "control.kb.v1.delete"
	SubjectKBList            = "control.kb.v1.list"
	SubjectPluginList        = "control.plugin.v1.list"
	SubjectPluginGet         = "control.plugin.v1.get"
	SubjectPluginInstall     = "control.plugin.v1.install"
	SubjectPluginUninstall   = "control.plugin.v1.uninstall"
	SubjectPluginSearch      = "control.plugin.v1.search"
	SubjectPluginHubInfo     = "control.plugin.v1.hub_info"
	SubjectServiceList       = "control.service.v1.list"
	SubjectServiceInspect    = "control.service.v1.inspect"
	SubjectServiceDoctor     = "control.service.v1.doctor"
	SubjectAuditGet          = "control.audit.v1.get"
	SubjectAuditList         = "control.audit.v1.list"
	SubjectAuditRetention    = "control.audit.v1.retention"
	SubjectRuntimeList       = "control.runtime.v1.list"
	SubjectRuntimeInspect    = "control.runtime.v1.inspect"
	SubjectArtifactInspect   = "control.artifact.v1.inspect"
)

const (
	SubjectSessionInputWildcard = "session.*.input"
	SubjectRuntimeInfoGet       = "runtime.info.v1.get"
	SubjectRuntimeSessionGet    = "runtime.session.v1.get"
	SubjectRuntimePlanGet       = "runtime.plan.v1.get"
	SubjectRuntimePlanApprove   = "runtime.plan.v1.approve"
	SubjectRuntimePlanReject    = "runtime.plan.v1.reject"
	SubjectRuntimeActivityList  = "runtime.activity.v1.list"
	SubjectRuntimeActivityFeed  = "runtime.activity.v1.events"
)

const (
	SubjectCatalogRuntimeGet    = "catalog.runtime.v1.get"
	SubjectCatalogRuntimeEvents = "catalog.runtime.v1.events"
)

func SessionInputSubject(sessionID string) (string, error) {
	session, err := token("session_id", sessionID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("session.%s.input", session), nil
}

func SessionEventsSubject(sessionID string) (string, error) {
	session, err := token("session_id", sessionID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("session.%s.events", session), nil
}

func SessionStatusSubject(sessionID string) (string, error) {
	session, err := token("session_id", sessionID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("session.%s.status", session), nil
}

func ArtifactDataSubject(artifactID string) (string, error) {
	artifact, err := token("artifact_id", artifactID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("artifact.%s.data", artifact), nil
}

func token(name, value string) (string, error) {
	out := stableToken(value)
	if out == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if !tokenPattern.MatchString(out) {
		return "", fmt.Errorf("%s %q is invalid", name, out)
	}
	return out, nil
}

func stableToken(value string) string {
	value = strings.TrimSpace(value)
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
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "id_" + out
	}
	return out
}
