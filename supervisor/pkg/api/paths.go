// Package api defines HTTP types and path helpers shared between the
// supervisor server, the supervisor Go SDK, and the runtime HTTP client.
package api

import (
	"fmt"
	"strings"
)

// DefaultRuntimeBasePath is the URL prefix for the direct runtime HTTP API.
const DefaultRuntimeBasePath = "/api/v1/runtime"

// Runtime API path suffixes (relative to a runtime's base URL).
const (
	PathHealth          = "/health"
	PathInfo            = "/info"
	PathMode            = "/mode"
	PathStats           = "/stats"
	PathChat            = "/chat"
	PathStop            = "/stop"
	PathPlan            = "/plan"
	PathPlanApprove     = "/plan/approve"
	PathPlanReject      = "/plan/reject"
	PathActivity        = "/activity"
	PathActivityStream  = "/activity/stream"
	PathSessions        = "/sessions"
	PathSession         = "/sessions/{sessionKey}"
	PathSessionActivity = "/sessions/{sessionKey}/activity"
	PathSessionBudget   = "/sessions/{sessionKey}/budget"
)

// Supervisor API route templates. Use the Route helpers below to construct
// concrete paths so the prefix and escaping rules live in one place.
const (
	routeSpaces          = "/v1/spaces"
	routeSpace           = "/v1/spaces/%s"
	routeSpaceConfig     = "/v1/spaces/%s/config"
	routeSpaceDoctor     = "/v1/spaces/%s/doctor"
	routeSpaceKBList     = "/v1/spaces/%s/kb/%s"
	routeSpaceKBItem     = "/v1/spaces/%s/kb/%s/%s"
	routeSpacePlugins    = "/v1/spaces/%s/plugins"
	routeSpacePlugin     = "/v1/spaces/%s/plugins/%s"
	routeSpacePluginSrch = "/v1/spaces/%s/plugins/search"
	routeSpaceHubPlugin  = "/v1/spaces/%s/plugins/hub/%s"
	routeSpaceServices   = "/v1/spaces/%s/services"
	routeSpaceService    = "/v1/spaces/%s/services/%s"
	routeServiceDoctor   = "/v1/spaces/%s/services/doctor"
	routeSpaceSessions   = "/v1/spaces/%s/sessions"
	routeSpaceSession    = "/v1/spaces/%s/sessions/%s"
	routeSpaceEventsStrm = "/v1/spaces/%s/events/stream"
	routeAgents          = "/v1/agents"
	routeAgent           = "/v1/agents/%s"
)

// Route returns concrete supervisor URL paths for the given space/agent ids.
// The returned paths are relative to the supervisor base URL.
func Route() RouteBuilder { return RouteBuilder{} }

// RouteBuilder produces supervisor-relative URL paths. Kept stateless so the
// same builder can be reused concurrently.
type RouteBuilder struct{}

func (RouteBuilder) Spaces() string                  { return routeSpaces }
func (RouteBuilder) Space(name string) string        { return fmt.Sprintf(routeSpace, name) }
func (RouteBuilder) SpaceConfig(n string) string     { return fmt.Sprintf(routeSpaceConfig, n) }
func (RouteBuilder) SpaceDoctor(n string) string     { return fmt.Sprintf(routeSpaceDoctor, n) }
func (RouteBuilder) SpaceKBList(n, ns string) string { return fmt.Sprintf(routeSpaceKBList, n, ns) }
func (RouteBuilder) SpaceKBItem(n, ns, k string) string {
	return fmt.Sprintf(routeSpaceKBItem, n, ns, k)
}
func (RouteBuilder) SpacePlugins(n string) string      { return fmt.Sprintf(routeSpacePlugins, n) }
func (RouteBuilder) SpacePlugin(n, name string) string { return fmt.Sprintf(routeSpacePlugin, n, name) }
func (RouteBuilder) SpacePluginSearch(n string) string { return fmt.Sprintf(routeSpacePluginSrch, n) }
func (RouteBuilder) SpaceHubPlugin(n, name string) string {
	return fmt.Sprintf(routeSpaceHubPlugin, n, name)
}
func (RouteBuilder) SpaceServices(n string) string { return fmt.Sprintf(routeSpaceServices, n) }
func (RouteBuilder) SpaceService(n, service string) string {
	return fmt.Sprintf(routeSpaceService, n, service)
}
func (RouteBuilder) SpaceServiceDoctor(n string) string { return fmt.Sprintf(routeServiceDoctor, n) }
func (RouteBuilder) SpaceSessions(n string) string      { return fmt.Sprintf(routeSpaceSessions, n) }
func (RouteBuilder) SpaceSession(n, id string) string {
	return fmt.Sprintf(routeSpaceSession, n, id)
}
func (RouteBuilder) SpaceEventStream(n string) string {
	return fmt.Sprintf(routeSpaceEventsStrm, n)
}
func (RouteBuilder) Agents() string         { return routeAgents }
func (RouteBuilder) Agent(id string) string { return fmt.Sprintf(routeAgent, id) }

// JoinPath concatenates a base path and a suffix, stripping trailing slashes.
func JoinPath(basePath, suffix string) string {
	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		return suffix
	}
	return basePath + suffix
}
