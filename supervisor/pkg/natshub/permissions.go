package natshub

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func toServerPermissions(config PermissionConfig) *natsserver.Permissions {
	if isEmptyPermissions(config) {
		return nil
	}
	return &natsserver.Permissions{
		Publish: &natsserver.SubjectPermission{
			Allow: append([]string(nil), config.PublishAllow...),
			Deny:  append([]string(nil), config.PublishDeny...),
		},
		Subscribe: &natsserver.SubjectPermission{
			Allow: append([]string(nil), config.SubscribeAllow...),
			Deny:  append([]string(nil), config.SubscribeDeny...),
		},
	}
}

func isEmptyPermissions(config PermissionConfig) bool {
	return len(config.PublishAllow) == 0 &&
		len(config.PublishDeny) == 0 &&
		len(config.SubscribeAllow) == 0 &&
		len(config.SubscribeDeny) == 0
}

func allowAllPermissions() PermissionConfig {
	return PermissionConfig{
		PublishAllow:   []string{">"},
		SubscribeAllow: []string{">"},
	}
}

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
			"catalog.runtime.v1.get",
			"session.*.events",
			"session.*.status",
			"runtime.activity.v1.events",
			"agent.*.events",
			"svc.>",
			"$JS.API.>",
			"$KV.>",
			"$O.>",
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			"catalog.runtime.v1.events",
			"session.*.input",
			"runtime.info.v1.get",
			"runtime.session.v1.get",
			"runtime.plan.v1.*",
			"runtime.activity.v1.list",
			"agent.*.invoke",
			"$JS.API.>",
			"$KV.>",
			"$O.>",
			"_INBOX.>",
			"_R_.>",
		},
	}
}

func UserPermissions() PermissionConfig {
	return PermissionConfig{
		PublishAllow: []string{
			"catalog.runtime.v1.get",
			"runtime.info.v1.get",
			"runtime.session.v1.get",
			"runtime.plan.v1.get",
			"runtime.plan.v1.approve",
			"runtime.plan.v1.reject",
			"runtime.activity.v1.list",
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			"catalog.runtime.v1.events",
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
			"audit.>",
			"telemetry.>",
			"$JS.API.>",
			"$KV.>",
			"$O.>",
			"_INBOX.>",
			"_R_.>",
		},
		SubscribeAllow: []string{
			"catalog.>",
			"control.>",
			"space.>",
			"svc.>",
			"audit.>",
			"telemetry.>",
			"$JS.API.>",
			"$KV.>",
			"$O.>",
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

// ServiceHostPermissions grants service hosts only their responder routes.
// Run State is the one current service that additionally owns a control-plane
// KV lease store and therefore needs JetStream request/reply access.
func ServiceHostPermissions(service string, routes []ServiceFunctionRoute) PermissionConfig {
	permissions := PermissionConfig{
		PublishAllow: []string{"_INBOX.>", "_R_.>"},
	}
	for _, route := range routes {
		permissions.SubscribeAllow = append(permissions.SubscribeAllow, route.ExportSubject)
	}
	if strings.TrimSpace(service) == "runstate" {
		permissions.PublishAllow = append(permissions.PublishAllow, "$JS.API.>", "$KV.>")
		permissions.SubscribeAllow = append(permissions.SubscribeAllow, "$JS.API.>", "$KV.>", "_INBOX.>", "_R_.>")
	}
	return permissions
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
	token = strings.ToLower(token)
	if token[0] >= '0' && token[0] <= '9' {
		token = "id_" + token
	}
	return token
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
