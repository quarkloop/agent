package natshub

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

func SpaceAccountName(spaceID string) (string, error) {
	token := stableToken(spaceID)
	if token == "" {
		return "", errors.New("space id is required")
	}
	return "SPACE_" + token, nil
}

func RuntimePermissions() PermissionConfig {
	return PermissionConfig{
		PublishAllow: []string{
			"session.*.events",
			"session.*.status",
			"runtime.activity.v1.events",
			"agent.*.events",
			"workflow.*.events",
			"svc.>",
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			"session.*.input",
			"runtime.plan.v1.*",
			"runtime.activity.v1.list",
			"agent.*.invoke",
			"_INBOX.>",
			"_R_.>",
		},
	}
}

func UserPermissions() PermissionConfig {
	return PermissionConfig{
		PublishAllow: []string{
			"runtime.plan.v1.get",
			"runtime.plan.v1.approve",
			"runtime.plan.v1.reject",
			"runtime.activity.v1.list",
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			"runtime.activity.v1.events",
			"_INBOX.>",
			"_R_.>",
		},
	}
}

func SupervisorPermissions() PermissionConfig {
	return PermissionConfig{
		PublishAllow: []string{
			"catalog.>",
			"control.>",
			"space.>",
			"svc.>",
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			"catalog.>",
			"control.>",
			"space.>",
			"svc.>",
			"_INBOX.>",
			"_R_.>",
		},
	}
}

func UserSessionPermissions(sessionID string) PermissionConfig {
	return SessionPermissions(sessionID)
}

func SessionPermissions(sessionID string) PermissionConfig {
	session := subjectToken(sessionID)
	return PermissionConfig{
		PublishAllow: []string{
			fmt.Sprintf("session.%s.input", session),
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			fmt.Sprintf("session.%s.events", session),
			fmt.Sprintf("session.%s.status", session),
			"_INBOX.>",
			"_R_.>",
		},
	}
}

func AgentPermissions(agentID string) PermissionConfig {
	agent := subjectToken(agentID)
	return PermissionConfig{
		PublishAllow: []string{
			fmt.Sprintf("agent.%s.events", agent),
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			fmt.Sprintf("agent.%s.invoke", agent),
			"_INBOX.>",
			"_R_.>",
		},
	}
}

func ServicePermissions() PermissionConfig {
	return PermissionConfig{
		PublishAllow: []string{
			"svc.>",
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			"svc.>",
			"_INBOX.>",
			"_R_.>",
		},
	}
}

func ObservabilityPermissions(spaceID string) PermissionConfig {
	space := subjectToken(spaceID)
	return PermissionConfig{
		PublishAllow: []string{
			fmt.Sprintf("telemetry.%s.>", space),
			fmt.Sprintf("audit.%s.>", space),
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			fmt.Sprintf("telemetry.%s.>", space),
			fmt.Sprintf("audit.%s.>", space),
			"_INBOX.>",
			"_R_.>",
		},
	}
}

func subjectToken(value string) string {
	token := stableToken(value)
	if token == "" {
		return "_"
	}
	return strings.ToLower(token)
}

func stableToken(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToUpper(r))
			lastUnderscore = false
		case r == '_' || r == '-' || r == '.':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	return out
}
