package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/quarkloop/supervisor/pkg/api"
)

// handleHealth serves GET /v1/health — liveness probe.
func (s *Server) handleHealth(c *fiber.Ctx) error {
	return c.JSON(api.HealthResponse{Status: "ok"})
}

func writeJSON(c *fiber.Ctx, status int, body any) error {
	return c.Status(status).JSON(body)
}

func writeError(c *fiber.Ctx, status int, msg string) error {
	return writeJSON(c, status, api.ErrorResponse{Error: msg})
}
