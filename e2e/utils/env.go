//go:build e2e

package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

type RuntimeIdentity struct {
	ID    string
	Space string
}

type E2EEnv struct {
	Root                string
	WorkingDir          string
	Space               string
	NATS                NATSEndpoints
	Agent               RuntimeIdentity
	Provider            string
	Model               string
	Embedding           GatewayEmbeddingOptions
	Services            []ServicePlugin
	RunningServices     []string
	Agents              []string
	ExtraServicePlugins []string
	Compose             *ComposeProject
}

type NATSEndpoints struct {
	ClientURL     string
	WebSocketURL  string
	MonitoringURL string
	StateDir      string
}

type StartOptions struct {
	// DisableKnowledgeServices omits the default Knowledge services for focused
	// scenarios that request only their declared service plugins.
	DisableKnowledgeServices bool
	// WorkingDir is an isolated user workspace bind-mounted into the services
	// that may read or intentionally mutate test fixtures.
	WorkingDir string
	// Embedding selects the configured real Gateway embedding model.
	Embedding GatewayEmbeddingOptions
	// Services declares service plugins exposed to runtime for the scenario.
	Services []ServicePlugin
	// ExtraServicePlugins installs descriptors without binding a running
	// service. It is retained only for catalog validation scenarios.
	ExtraServicePlugins []string
	// Agents declares installed/enabled agent profiles.
	Agents []string
	// AgentServicePermissions narrows each enabled profile to service functions.
	AgentServicePermissions map[string][]string
}

// StartE2E provisions one isolated Docker Compose deployment, creates its
// space through the supervisor NATS contract, then starts the NATS-only
// runtime with a space-scoped credential.
func StartE2E(t *testing.T, withProvider bool, opts ...StartOptions) *E2EEnv {
	t.Helper()

	var opt StartOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	embedding := opt.Embedding.withDefaults()
	var cfg ProviderConfig
	if withProvider {
		cfg = RequireProviderConfig(t)
	}
	var provider, model string
	if withProvider {
		provider = cfg.Provider
		model = cfg.Model
	}
	workingDir := opt.WorkingDir
	if workingDir == "" {
		workingDir = t.TempDir()
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("mkdir working dir: %v", err)
	}
	if err := os.Chmod(workingDir, 0o755); err != nil {
		t.Fatalf("make bind-mounted working directory readable: %v", err)
	}

	project := NewComposeProject(t, workingDir)
	projectEnv := make(map[string]string)
	if withProvider {
		projectEnv["QUARK_MODEL_PROVIDER"] = provider
		projectEnv["QUARK_MODEL_NAME"] = model
		projectEnv["OPENROUTER_MODEL"] = model
		projectEnv["QUARK_GATEWAY_MAX_EXTERNAL_REQUESTS"] = strconv.FormatInt(int64Env("QUARK_E2E_MAX_PROVIDER_REQUESTS", 50), 10)
	}
	if embedding.Provider != "" {
		projectEnv["QUARK_GATEWAY_EMBEDDING_PROVIDER"] = embedding.Provider
	}
	if embedding.Model != "" {
		projectEnv["OPENROUTER_EMBEDDING_MODEL"] = embedding.Model
	}
	project.SetEnv(projectEnv)
	project.Up("supervisor", "space")
	natsEndpoints := project.Endpoints()
	waitForControlNATS(t, natsEndpoints, 45*time.Second)

	spaceName := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	agents := withDefaultMainAgent(opt.Agents)
	createSpace(t, natsEndpoints, clientcontract.CreateSpaceRequest{
		Config: spaceConfigFor(t, spaceName, workingDir, provider, model, opt.Services, opt.ExtraServicePlugins, agents, opt.AgentServicePermissions, !opt.DisableKnowledgeServices),
	})
	runtimeCredential := issueRuntimeCredential(t, natsEndpoints, spaceName)
	runtimeID := "e2e-runtime-" + spaceName
	project.SetEnv(map[string]string{
		"QUARK_RUNTIME_ID":            runtimeID,
		"QUARK_SPACE":                 spaceName,
		"QUARK_RUNTIME_NATS_USER":     runtimeCredential.Username,
		"QUARK_RUNTIME_NATS_PASSWORD": runtimeCredential.Password,
	})

	serviceContainers := composeServicesFor(opt, withProvider)
	env := &E2EEnv{
		Root:                QuarkRoot(t),
		WorkingDir:          workingDir,
		Space:               spaceName,
		NATS:                natsEndpoints,
		Provider:            provider,
		Model:               model,
		Embedding:           embedding,
		Services:            append([]ServicePlugin(nil), opt.Services...),
		RunningServices:     append([]string(nil), serviceContainers...),
		Agents:              append([]string(nil), agents...),
		ExtraServicePlugins: append([]string(nil), opt.ExtraServicePlugins...),
		Compose:             project,
		Agent:               RuntimeIdentity{ID: runtimeID, Space: spaceName},
	}

	project.Up(composeStartupContainers(serviceContainers)...)
	waitForServiceResponders(t, env, serviceContainers, 60*time.Second)
	if withProvider {
		preflightGateway(t, env)
	}
	DumpNATSCLIDiagnostics(t, env.NATS, "after-services", env.RunningServices, project.ArtifactsDir())
	project.Up("runtime")
	waitForRuntimeNATS(t, env, 45*time.Second)
	DumpNATSCLIDiagnostics(t, env.NATS, "after-runtime", env.RunningServices, project.ArtifactsDir())
	Logf(t, "runtime=%s nats=%s space=%s services=%s", runtimeID, natsEndpoints.ClientURL, spaceName, strings.Join(serviceContainers, ","))
	return env
}

func composeServicesFor(opt StartOptions, withProvider bool) []string {
	// IO supplies baseline workspace access and Harness supplies mandatory
	// runtime context composition for every inference-capable scenario.
	services := []string{"io", "harness"}
	if !opt.DisableKnowledgeServices {
		services = append(services, "core", "gateway", "indexer", "document", "runstate", "citation")
	}
	for _, service := range opt.Services {
		services = append(services, service.withDefaults().Name)
	}
	if withProvider {
		services = append(services, "gateway")
	}
	return uniqueNonEmpty(services)
}

// composeStartupContainers includes infrastructure that supplies a declared
// Quark service but is not itself a service-function responder.
func composeStartupContainers(services []string) []string {
	startup := append([]string(nil), services...)
	for _, service := range services {
		switch service {
		case "indexer":
			startup = append(startup, "dgraph")
		case "secrets":
			startup = append(startup, "openbao")
		case "workflow":
			startup = append(startup, "temporal")
		}
	}
	return uniqueNonEmpty(startup)
}

func withDefaultMainAgent(agents []string) []string {
	const mainAgent = "quark-main"
	out := make([]string, 0, len(agents)+1)
	seen := make(map[string]struct{}, len(agents)+1)
	add := func(agent string) {
		agent = strings.TrimSpace(agent)
		if agent == "" {
			return
		}
		if _, ok := seen[agent]; ok {
			return
		}
		seen[agent] = struct{}{}
		out = append(out, agent)
	}
	add(mainAgent)
	for _, agent := range agents {
		add(agent)
	}
	return out
}
