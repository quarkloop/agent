package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/runtime"
	"github.com/quarkloop/supervisor/pkg/runtime/launchenv"
	"github.com/quarkloop/supervisor/pkg/space/store"
)

// handleListAgents serves GET /v1/agents.
func (s *Server) handleListAgents(c *fiber.Ctx) error {
	agents := s.registry.List()
	out := make([]api.RuntimeInfo, 0, len(agents))
	for _, a := range agents {
		out = append(out, toAPIRuntimeInfo(a))
	}
	return writeJSON(c, fiber.StatusOK, out)
}

// handleStartRuntime serves POST /v1/agents.
func (s *Server) handleStartRuntime(c *fiber.Ctx) error {
	var req api.StartRuntimeRequest
	if err := c.BodyParser(&req); err != nil {
		return writeError(c, fiber.StatusBadRequest, "invalid request body: "+err.Error())
	}
	if req.Space == "" {
		return writeError(c, fiber.StatusBadRequest, "space is required")
	}

	port := req.Port
	if port == 0 {
		p, err := reservePort()
		if err != nil {
			return writeError(c, fiber.StatusInternalServerError, "reserve port: "+err.Error())
		}
		port = p
	}

	sp, err := s.store.Get(req.Space)
	if err != nil {
		if store.IsNotFound(err) {
			return writeError(c, fiber.StatusNotFound, fmt.Sprintf("space %q not found", req.Space))
		}
		return writeError(c, fiber.StatusInternalServerError, err.Error())
	}
	if existing, err := s.registry.GetBySpace(req.Space); err == nil {
		return writeError(c, fiber.StatusConflict, fmt.Sprintf("runtime %s already running for space %q", existing.ID(), req.Space))
	}

	pluginsDir := ""
	if mgr, err := s.store.Plugins(req.Space); err == nil {
		pluginsDir = mgr.PluginsDir()
	}

	agent := runtime.NewRuntime(generateRuntimeID(), req.Space, sp.WorkingDir, pluginsDir)
	agent.SetPort(port)
	s.registry.Register(agent)

	env, err := s.store.AgentEnvironment(req.Space)
	if err != nil {
		s.registry.Remove(agent.ID())
		return writeError(c, fiber.StatusBadRequest, err.Error())
	}
	pluginCatalogEnv, err := s.runtimePluginCatalogEnv(c.Context(), req.Space)
	if err != nil {
		s.registry.Remove(agent.ID())
		return writeError(c, fiber.StatusBadRequest, err.Error())
	}
	env = append(env, pluginCatalogEnv...)
	catalogEnv, err := s.runtimeServiceCatalogEnv(c.Context(), req.Space)
	if err != nil {
		s.registry.Remove(agent.ID())
		return writeError(c, fiber.StatusBadRequest, err.Error())
	}
	spec, err := s.launchEnv.Build(launchenv.Inputs{
		RuntimeID:  agent.ID(),
		Space:      agent.Space(),
		WorkingDir: agent.WorkingDir(),
		Port:       agent.Port(),
		PluginsDir: agent.PluginsDir(),
		RuntimeEnv: env,
		CatalogEnv: append(pluginCatalogEnv, catalogEnv...),
	})
	if err != nil {
		s.registry.Remove(agent.ID())
		return writeError(c, fiber.StatusBadRequest, err.Error())
	}

	process, err := s.launcher.Start(c.Context(), agent.ID(), spec)
	if err != nil {
		s.registry.Remove(agent.ID())
		return writeError(c, fiber.StatusInternalServerError, err.Error())
	}
	if err := s.registry.MarkRunning(agent.ID(), process); err != nil {
		if process.Cmd != nil && process.Cmd.Process != nil {
			_ = process.Cmd.Process.Kill()
		}
		s.registry.Remove(agent.ID())
		return writeError(c, fiber.StatusInternalServerError, err.Error())
	}

	return writeJSON(c, fiber.StatusCreated, toAPIRuntimeInfo(agent))
}

// handleGetRuntime serves GET /v1/runtimes/:id.
func (s *Server) handleGetRuntime(c *fiber.Ctx) error {
	id := c.Params("id")
	agent, err := s.registry.Get(id)
	if err != nil {
		return writeError(c, fiber.StatusNotFound, err.Error())
	}
	return writeJSON(c, fiber.StatusOK, toAPIRuntimeInfo(agent))
}

// handleStopRuntime serves POST /v1/runtimes/:id/stop.
func (s *Server) handleStopRuntime(c *fiber.Ctx) error {
	id := c.Params("id")

	agent, err := s.registry.Get(id)
	if err != nil {
		return writeError(c, fiber.StatusNotFound, err.Error())
	}
	if agent.Status() != api.RuntimeRunning && agent.Status() != api.RuntimeStarting {
		return writeError(c, fiber.StatusConflict, fmt.Sprintf("runtime %s is not running (status: %s)", id, agent.Status()))
	}
	if err := s.registry.MarkStopping(id); err != nil {
		return writeError(c, fiber.StatusConflict, err.Error())
	}
	if err := s.launcher.Stop(agent); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err.Error())
	}
	return writeJSON(c, fiber.StatusOK, toAPIRuntimeInfo(agent))
}

// generateRuntimeID returns a short random hex ID.
func generateRuntimeID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback: use timestamp + pid for uniqueness (not cryptographically secure)
		slog.Error("failed to generate random ID", "error", err)
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
