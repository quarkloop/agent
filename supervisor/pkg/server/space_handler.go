package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/space"
	"github.com/quarkloop/supervisor/pkg/space/store"
)

// handleListSpaces serves GET /v1/spaces.
func (s *Server) handleListSpaces(c *fiber.Ctx) error {
	spaces, err := s.store.List()
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err.Error())
	}
	out := make([]api.SpaceInfo, 0, len(spaces))
	for _, sp := range spaces {
		out = append(out, toSpaceInfo(sp))
	}
	return writeJSON(c, fiber.StatusOK, out)
}

// handleCreateSpace serves POST /v1/spaces.
func (s *Server) handleCreateSpace(c *fiber.Ctx) error {
	var req api.CreateSpaceRequest
	if err := c.BodyParser(&req); err != nil {
		return writeError(c, fiber.StatusBadRequest, "invalid request body: "+err.Error())
	}

	if len(req.Config) == 0 {
		return writeError(c, fiber.StatusBadRequest, "config is required")
	}
	if _, err := spacemodel.ParseConfig(req.Config); err != nil {
		return writeError(c, fiber.StatusBadRequest, err.Error())
	}

	sp, err := s.store.Create(cloneBytes(req.Config))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrAlreadyExists):
			return writeError(c, fiber.StatusConflict, err.Error())
		default:
			return writeError(c, fiber.StatusBadRequest, err.Error())
		}
	}
	if err := s.BootstrapSpace(context.Background(), sp.Name); err != nil {
		if rollbackErr := s.store.Delete(sp.Name); rollbackErr != nil {
			return writeError(c, fiber.StatusInternalServerError, fmt.Sprintf("bootstrap required space plugins: %v; rollback space: %v", err, rollbackErr))
		}
		return writeError(c, fiber.StatusInternalServerError, "bootstrap required space plugins: "+err.Error())
	}
	if s.natsHub != nil {
		if _, err := s.natsHub.ProvisionSpace(sp.Name); err != nil {
			if rollbackErr := s.store.Delete(sp.Name); rollbackErr != nil {
				return writeError(c, fiber.StatusInternalServerError, fmt.Sprintf("provision nats space account: %v; rollback space: %v", err, rollbackErr))
			}
			return writeError(c, fiber.StatusInternalServerError, "provision nats space account: "+err.Error())
		}
	}

	return writeJSON(c, fiber.StatusCreated, toSpaceInfo(sp))
}

// handleGetSpace serves GET /v1/spaces/:name.
func (s *Server) handleGetSpace(c *fiber.Ctx) error {
	name := c.Params("name")
	sp, err := s.store.Get(name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}
	return writeJSON(c, fiber.StatusOK, toSpaceInfo(sp))
}

// handleDeleteSpace serves DELETE /v1/spaces/:name.
func (s *Server) handleDeleteSpace(c *fiber.Ctx) error {
	name := c.Params("name")
	if err := s.store.Delete(name); err != nil {
		return s.writeSpaceError(c, name, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleGetSpaceConfig serves GET /v1/spaces/:name/config.
func (s *Server) handleGetSpaceConfig(c *fiber.Ctx) error {
	name := c.Params("name")
	data, err := s.store.Config(name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}
	sp, err := s.store.Get(name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}

	return writeJSON(c, fiber.StatusOK, api.SpaceConfigResponse{
		Name:      name,
		Version:   sp.Version,
		Config:    data,
		UpdatedAt: sp.UpdatedAt,
	})
}

// handleUpdateSpaceConfig serves PUT /v1/spaces/:name/config.
func (s *Server) handleUpdateSpaceConfig(c *fiber.Ctx) error {
	name := c.Params("name")
	var req api.UpdateSpaceConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return writeError(c, fiber.StatusBadRequest, "invalid request body: "+err.Error())
	}
	if len(req.Config) == 0 {
		return writeError(c, fiber.StatusBadRequest, "config is required")
	}
	cfg, err := spacemodel.ParseConfig(req.Config)
	if err != nil {
		return writeError(c, fiber.StatusBadRequest, err.Error())
	}
	if cfg.Name != name {
		return writeError(c, fiber.StatusBadRequest, "config name does not match route space")
	}
	sp, err := s.store.UpdateConfig(cloneBytes(req.Config))
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}
	return writeJSON(c, fiber.StatusOK, toSpaceInfo(sp))
}

// handleDoctor serves POST /v1/spaces/:name/doctor.
func (s *Server) handleDoctor(c *fiber.Ctx) error {
	name := c.Params("name")
	resp, err := s.store.Doctor(name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}
	return writeJSON(c, fiber.StatusOK, resp)
}

// writeSpaceError maps space errors to HTTP status codes.
func (s *Server) writeSpaceError(c *fiber.Ctx, name string, err error) error {
	switch {
	case store.IsNotFound(err):
		return writeError(c, fiber.StatusNotFound, fmt.Sprintf("space %q not found", name))
	case errors.Is(err, store.ErrAlreadyExists):
		return writeError(c, fiber.StatusConflict, err.Error())
	default:
		return writeError(c, fiber.StatusInternalServerError, err.Error())
	}
}

func toSpaceInfo(sp *space.Space) api.SpaceInfo {
	return api.SpaceInfo{
		Name:       sp.Name,
		Version:    sp.Version,
		WorkingDir: sp.WorkingDir,
		CreatedAt:  sp.CreatedAt,
		UpdatedAt:  sp.UpdatedAt,
	}
}
