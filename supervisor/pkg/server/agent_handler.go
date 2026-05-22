package server

import "github.com/gofiber/fiber/v2"

const deploymentManagedRuntimeMessage = "runtime lifecycle is deployment-managed; start runtime processes with Docker Compose, systemd, or another operator-owned launcher"

func (s *Server) handleListAgents(c *fiber.Ctx) error {
	return writeJSON(c, fiber.StatusOK, []any{})
}

func (s *Server) handleGetRuntime(c *fiber.Ctx) error {
	return writeError(c, fiber.StatusNotFound, deploymentManagedRuntimeMessage)
}
