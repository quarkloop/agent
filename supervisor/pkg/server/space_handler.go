package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
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

	if req.Name == "" {
		return writeError(c, fiber.StatusBadRequest, "name is required")
	}
	if len(req.Quarkfile) == 0 {
		return writeError(c, fiber.StatusBadRequest, "quarkfile is required")
	}
	if req.WorkingDir == "" {
		return writeError(c, fiber.StatusBadRequest, "working_dir is required")
	}

	sp, err := s.store.Create(req.Name, cloneBytes(req.Quarkfile), req.WorkingDir)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrAlreadyExists):
			return writeError(c, fiber.StatusConflict, err.Error())
		default:
			return writeError(c, fiber.StatusBadRequest, err.Error())
		}
	}
	if err := s.BootstrapSpace(context.Background(), req.Name); err != nil {
		if rollbackErr := s.store.Delete(req.Name); rollbackErr != nil {
			return writeError(c, fiber.StatusInternalServerError, fmt.Sprintf("bootstrap required space plugins: %v; rollback space: %v", err, rollbackErr))
		}
		return writeError(c, fiber.StatusInternalServerError, "bootstrap required space plugins: "+err.Error())
	}
	if s.natsHub != nil {
		if _, err := s.natsHub.ProvisionSpace(req.Name); err != nil {
			if rollbackErr := s.store.Delete(req.Name); rollbackErr != nil {
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

// handleGetQuarkfile serves GET /v1/spaces/:name/quarkfile.
func (s *Server) handleGetQuarkfile(c *fiber.Ctx) error {
	name := c.Params("name")
	data, err := s.store.Quarkfile(name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}
	sp, err := s.store.Get(name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}

	return writeJSON(c, fiber.StatusOK, api.QuarkfileResponse{
		Name:      name,
		Version:   sp.Version,
		Quarkfile: data,
		UpdatedAt: sp.UpdatedAt,
	})
}

// handleUpdateQuarkfile serves PUT /v1/spaces/:name/quarkfile.
func (s *Server) handleUpdateQuarkfile(c *fiber.Ctx) error {
	name := c.Params("name")
	var req api.UpdateQuarkfileRequest
	if err := c.BodyParser(&req); err != nil {
		return writeError(c, fiber.StatusBadRequest, "invalid request body: "+err.Error())
	}
	if len(req.Quarkfile) == 0 {
		return writeError(c, fiber.StatusBadRequest, "quarkfile is required")
	}
	sp, err := s.store.UpdateQuarkfile(name, cloneBytes(req.Quarkfile))
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
