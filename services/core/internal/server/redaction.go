package server

import (
	"strings"

	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
)

const redactedJSON = `{"redacted":true}`

var sensitiveMarkers = []string{
	"api_key",
	"apikey",
	"authorization",
	"bearer ",
	"client_secret",
	"credential",
	"jwt",
	"oauth",
	"password",
	"private_key",
	"refresh_token",
	"secret",
	"token",
}

func redactAuditEvent(event *corev1.AuditEvent) {
	reasons := redactionReasons(event.GetActor(), event.GetAction(), event.GetTarget())
	if len(reasons) == 0 {
		return
	}
	event.Redacted = true
	event.RedactionReasons = mergeReasons(event.GetRedactionReasons(), reasons)
	event.Actor = redactText(event.GetActor())
	event.Action = redactText(event.GetAction())
	event.Target = redactText(event.GetTarget())
}

func redactArtifact(artifact *corev1.Artifact) {
	reasons := redactionReasons(artifact.GetKind(), artifact.GetUri(), artifact.GetDigest())
	if len(reasons) == 0 {
		return
	}
	artifact.Redacted = true
	artifact.RedactionReasons = mergeReasons(artifact.GetRedactionReasons(), reasons)
	artifact.Uri = redactText(artifact.GetUri())
	artifact.Digest = redactText(artifact.GetDigest())
}

func redactConfig(value *corev1.ConfigValue) {
	reasons := redactionReasons(value.GetScope(), value.GetKey(), value.GetValueJson())
	if len(reasons) == 0 {
		return
	}
	value.Redacted = true
	value.ValueJson = redactedJSON
}

func redactEvent(event *corev1.Event) {
	reasons := redactionReasons(event.GetStream(), event.GetKind(), event.GetPayloadJson())
	if len(reasons) == 0 {
		return
	}
	event.Redacted = true
	event.PayloadJson = redactedJSON
}

func redactionReasons(values ...string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, value := range values {
		normalized := strings.ToLower(value)
		for _, marker := range sensitiveMarkers {
			if strings.Contains(normalized, marker) {
				reason := "contains-" + strings.TrimSpace(strings.ReplaceAll(marker, "_", "-"))
				if _, ok := seen[reason]; ok {
					continue
				}
				seen[reason] = struct{}{}
				out = append(out, reason)
			}
		}
	}
	return out
}

func redactText(value string) string {
	if value == "" {
		return value
	}
	if len(redactionReasons(value)) == 0 {
		return value
	}
	return "[redacted]"
}

func mergeReasons(existing, next []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(next))
	out := make([]string, 0, len(existing)+len(next))
	for _, reason := range append(existing, next...) {
		if reason == "" {
			continue
		}
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		out = append(out, reason)
	}
	return out
}
