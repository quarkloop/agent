package server

// routes registers all supervisor API routes on the Fiber app.
func (s *Server) routes() {
	app := s.app

	// Health
	app.Get("/v1/health", s.handleHealth)

	// Space data CRUD
	app.Get("/v1/spaces", s.handleListSpaces)
	app.Post("/v1/spaces", s.handleCreateSpace)
	app.Get("/v1/spaces/:name", s.handleGetSpace)
	app.Delete("/v1/spaces/:name", s.handleDeleteSpace)

	// Quarkfile
	app.Get("/v1/spaces/:name/quarkfile", s.handleGetQuarkfile)
	app.Put("/v1/spaces/:name/quarkfile", s.handleUpdateQuarkfile)

	// Doctor
	app.Post("/v1/spaces/:name/doctor", s.handleDoctor)

	// KB
	app.Get("/v1/spaces/:name/kb/:namespace", s.handleKBList)
	app.Get("/v1/spaces/:name/kb/:namespace/:key", s.handleKBGet)
	app.Put("/v1/spaces/:name/kb/:namespace/:key", s.handleKBSet)
	app.Delete("/v1/spaces/:name/kb/:namespace/:key", s.handleKBDelete)

	// Plugins
	app.Get("/v1/spaces/:name/plugins", s.handleListPlugins)
	app.Post("/v1/spaces/:name/plugins", s.handleInstallPlugin)
	app.Get("/v1/spaces/:name/plugins/search", s.handleSearchPlugins)
	app.Get("/v1/spaces/:name/plugins/hub/:plugin", s.handleHubPluginInfo)
	app.Get("/v1/spaces/:name/plugins/:plugin", s.handleGetPlugin)
	app.Delete("/v1/spaces/:name/plugins/:plugin", s.handleUninstallPlugin)

	// Services
	app.Get("/v1/spaces/:name/services", s.handleListServices)
	app.Post("/v1/spaces/:name/services/doctor", s.handleServiceDoctor)
	app.Get("/v1/spaces/:name/services/:service", s.handleInspectService)
	app.Get("/v1/spaces/:name/services/:service/logs", s.handleServiceLogs)
	app.Post("/v1/spaces/:name/services/:service/restart", s.handleRestartService)

	// Sessions
	app.Get("/v1/spaces/:name/sessions", s.handleListSessions)
	app.Post("/v1/spaces/:name/sessions", s.handleCreateSession)
	app.Get("/v1/spaces/:name/sessions/:id", s.handleGetSession)
	app.Delete("/v1/spaces/:name/sessions/:id", s.handleDeleteSession)

	// Supervisor → agent event stream
	app.Get("/v1/spaces/:name/events/stream", s.handleEventStream)

	// Agents (runtime)
	app.Get("/v1/agents", s.handleListAgents)
	app.Post("/v1/agents", s.handleStartRuntime)
	app.Get("/v1/agents/:id", s.handleGetRuntime)
	app.Post("/v1/agents/:id/stop", s.handleStopRuntime)
}
