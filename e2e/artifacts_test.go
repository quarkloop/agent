//go:build e2e

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/pkg/boundary/redaction"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
)

type agentRunArtifacts struct {
	Reply         string
	Tools         string
	ToolEvents    string
	Observability string
}

func writeAgentRunArtifacts(t *testing.T, dir, prefix string, env *utils.E2EEnv, trace utils.MessageTrace, prompt string) agentRunArtifacts {
	t.Helper()
	artifacts := agentRunArtifacts{
		Reply:         filepath.Join(dir, prefix+"-reply.txt"),
		Tools:         filepath.Join(dir, prefix+"-tools.txt"),
		ToolEvents:    filepath.Join(dir, prefix+"-tool-events.json"),
		Observability: filepath.Join(dir, prefix+"-observability.json"),
	}
	writeArtifact(t, dir, prefix+"-reply.txt", trace.Text)
	writeArtifact(t, dir, prefix+"-tools.txt", strings.Join(trace.ToolStarts, "\n"))
	writeTraceArtifact(t, dir, prefix+"-tool-events.json", trace)
	writeJSONArtifact(t, dir, prefix+"-observability.json", map[string]any{
		"artifact_id": prefix + "-observability",
		"space":       env.Space,
		"session_id":  trace.SessionID,
		"run_id":      trace.RunID,
		"runtime_id":  env.Agent.ID,
		"supervisor":  env.SupURL,
		"nats": map[string]any{
			"client_url":     env.NATS.ClientURL,
			"websocket_url":  env.NATS.WebSocketURL,
			"monitoring_url": env.NATS.MonitoringURL,
		},
		"prompt_sha256": promptHash(prompt),
		"prompt": map[string]any{
			"preview": previewPrompt(prompt, 500),
			"bytes":   len(prompt),
		},
		"model": map[string]any{
			"provider": env.Provider,
			"name":     env.Model,
		},
		"embedding": map[string]any{
			"plugin":     env.Embedding.Plugin,
			"mode":       env.Embedding.Mode,
			"provider":   env.Embedding.Provider,
			"model":      env.Embedding.Model,
			"dimensions": env.Embedding.Dimensions,
		},
		"catalog_snapshot": catalogSnapshot(env, trace),
		"profile_snapshot": profileSnapshot(env),
		"model_usage":      modelUsageSnapshot(env, trace),
		"model_usage_timeline": []map[string]any{
			modelUsageSnapshot(env, trace),
		},
		"services":         serviceSnapshot(env),
		"tool_timeline":    toolTimeline(trace),
		"service_timeline": serviceTimeline(trace),
		"diagnostics":      diagnosticsSnapshot(trace),
		"artifacts": map[string]string{
			"reply":         artifacts.Reply,
			"tools":         artifacts.Tools,
			"tool_events":   artifacts.ToolEvents,
			"observability": artifacts.Observability,
		},
	})
	return artifacts
}

func writeArtifact(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(redactString(content)), 0o644); err != nil {
		t.Fatalf("write artifact %s: %v", path, err)
	}
	utils.Logf(t, "manual verification artifact: %s", path)
	return path
}

func writeTraceArtifact(t *testing.T, dir, name string, trace utils.MessageTrace) {
	t.Helper()
	payload := map[string]any{
		"text":    trace.Text,
		"starts":  trace.ToolStartEvents,
		"results": trace.ToolResultEvents,
	}
	writeJSONArtifact(t, dir, name, payload)
}

func writeJSONArtifact(t *testing.T, dir, name string, payload any) {
	t.Helper()
	data, err := json.MarshalIndent(redactValue(payload), "", "  ")
	if err != nil {
		t.Fatalf("marshal artifact %s: %v", name, err)
	}
	writeArtifact(t, dir, name, string(data))
}

func promptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

func previewPrompt(prompt string, limit int) string {
	if len(prompt) <= limit {
		return prompt
	}
	return prompt[:limit] + "...(truncated)"
}

func serviceSnapshot(env *utils.E2EEnv) []map[string]any {
	services := []map[string]any{{
		"name":       "embedding",
		"plugin":     env.Embedding.Plugin,
		"mode":       env.Embedding.Mode,
		"provider":   env.Embedding.Provider,
		"model":      env.Embedding.Model,
		"dimensions": env.Embedding.Dimensions,
	}}
	for _, service := range env.Services {
		service = service.WithDefaults()
		services = append(services, map[string]any{
			"name":        service.Name,
			"plugin":      service.Plugin,
			"mode":        service.Mode,
			"address_env": service.AddressEnv,
		})
	}
	return services
}

func catalogSnapshot(env *utils.E2EEnv, trace utils.MessageTrace) map[string]any {
	return map[string]any{
		"space":       env.Space,
		"services":    serviceSnapshot(env),
		"tool_names":  uniqueStrings(trace.ToolStarts),
		"runtime_id":  env.Agent.ID,
		"supervisor":  env.SupURL,
		"catalog_ref": "supervisor-resolved-runtime-catalog",
	}
}

func profileSnapshot(env *utils.E2EEnv) map[string]any {
	return map[string]any{
		"provider":         env.Provider,
		"model":            env.Model,
		"embedding_plugin": env.Embedding.Plugin,
		"embedding_model":  env.Embedding.Model,
	}
}

func modelUsageSnapshot(env *utils.E2EEnv, trace utils.MessageTrace) map[string]any {
	return map[string]any{
		"provider":          env.Provider,
		"model":             env.Model,
		"tool_calls":        len(trace.ToolStartEvents),
		"reported_by_model": false,
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func toolTimeline(trace utils.MessageTrace) []map[string]any {
	timeline := make([]map[string]any, 0, len(trace.ToolStartEvents)+len(trace.ToolResultEvents))
	for i, event := range trace.ToolStartEvents {
		timeline = append(timeline, map[string]any{
			"sequence":        i,
			"phase":           "start",
			"call_id":         event.CallID,
			"service_call_id": firstNonEmpty(event.ServiceCallID, event.CallID),
			"name":            event.Name,
			"arguments":       event.Arguments,
			"session_id":      event.SessionID,
			"run_id":          event.RunID,
			"observed_at":     event.ObservedAt,
		})
	}
	offset := len(timeline)
	for i, event := range trace.ToolResultEvents {
		timeline = append(timeline, map[string]any{
			"sequence":        offset + i,
			"phase":           "result",
			"call_id":         event.CallID,
			"service_call_id": firstNonEmpty(event.ServiceCallID, event.CallID),
			"name":            event.Name,
			"error":           event.Error,
			"result":          event.Result,
			"duration_millis": event.DurationMillis,
			"session_id":      event.SessionID,
			"run_id":          event.RunID,
			"observed_at":     event.ObservedAt,
		})
	}
	return timeline
}

func serviceTimeline(trace utils.MessageTrace) []map[string]any {
	timeline := make([]map[string]any, 0)
	for _, event := range append(append([]utils.ToolEvent(nil), trace.ToolStartEvents...), trace.ToolResultEvents...) {
		service, ok := serviceNameFromTool(event.Name)
		if !ok {
			continue
		}
		entry := map[string]any{
			"service":         service,
			"tool":            event.Name,
			"call_id":         event.CallID,
			"service_call_id": firstNonEmpty(event.ServiceCallID, event.CallID),
			"error":           event.Error,
			"duration_millis": event.DurationMillis,
			"session_id":      event.SessionID,
			"run_id":          event.RunID,
			"observed_at":     event.ObservedAt,
		}
		if subject := serviceSubjectFromTool(event.Name); subject != "" {
			entry["subject"] = subject
		}
		timeline = append(timeline, entry)
	}
	return timeline
}

func diagnosticsSnapshot(trace utils.MessageTrace) []map[string]any {
	diagnostics := make([]map[string]any, 0)
	for _, event := range trace.ToolResultEvents {
		if !event.Error {
			continue
		}
		service, _ := serviceNameFromTool(event.Name)
		diagnostics = append(diagnostics, map[string]any{
			"code":            "tool.call_failed",
			"severity":        "error",
			"service":         service,
			"tool":            event.Name,
			"call_id":         event.CallID,
			"service_call_id": firstNonEmpty(event.ServiceCallID, event.CallID),
			"session_id":      event.SessionID,
			"run_id":          event.RunID,
			"message":         "Tool or service function returned an error.",
		})
	}
	return diagnostics
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func serviceNameFromTool(name string) (string, bool) {
	before, _, ok := strings.Cut(name, "_")
	if !ok || before == "" {
		return "", false
	}
	return before, true
}

func serviceSubjectFromTool(name string) string {
	service, ok := serviceNameFromTool(name)
	if !ok {
		return ""
	}
	_, function, ok := strings.Cut(name, "_")
	if !ok {
		return ""
	}
	subject, err := servicefunction.Subject(service, servicefunction.DefaultVersion, function)
	if err != nil {
		return ""
	}
	return subject
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case string:
		return redactString(typed)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			out[key] = redactValue(nested)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, nested := range typed {
			out[key] = redactString(nested)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, 0, len(typed))
		for _, nested := range typed {
			out = append(out, redactValue(nested).(map[string]any))
		}
		return out
	case []utils.ToolEvent:
		out := make([]utils.ToolEvent, 0, len(typed))
		for _, nested := range typed {
			out = append(out, redactValue(nested).(utils.ToolEvent))
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, nested := range typed {
			out = append(out, redactValue(nested))
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, nested := range typed {
			out = append(out, redactString(nested))
		}
		return out
	case utils.ToolEvent:
		typed.Arguments = redactString(typed.Arguments)
		typed.Result = redactString(typed.Result)
		return typed
	default:
		return value
	}
}

func redactString(value string) string {
	return redaction.RedactString(value)
}
