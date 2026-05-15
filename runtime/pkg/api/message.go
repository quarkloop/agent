package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/quarkloop/runtime/pkg/message"
)

// MessageHandler holds message handler dependencies.
type MessageHandler struct {
	poster   message.Poster
	sessions message.SessionAccess
}

const messageStreamHeartbeatInterval = 15 * time.Second

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(p message.Poster, sa message.SessionAccess) *MessageHandler {
	return &MessageHandler{poster: p, sessions: sa}
}

// RegisterRoutes wires message routes onto the given Fiber router.
// The group is expected to be mounted at /v1/sessions/:session_id/messages.
func (h *MessageHandler) RegisterRoutes(g fiber.Router) {
	g.Get("", h.List)
	g.Post("", h.Send)
	g.Get("/stream", h.Stream)
	g.Patch("/:message_id", h.Edit)
}

// List handles GET /v1/sessions/:session_id/messages.
func (h *MessageHandler) List(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")
	if !h.sessions.Has(sessionID) {
		return c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: "session not found"})
	}
	return c.JSON(mapMessageResponses(h.sessions.GetMessages(sessionID)))
}

// Send handles POST /v1/sessions/:session_id/messages (SSE streaming response).
func (h *MessageHandler) Send(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")

	var req sendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}

	if !h.sessions.Has(sessionID) {
		return c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: "session not found"})
	}

	resp := make(chan message.StreamMessage, 64)
	postCtx, cancelPost := context.WithCancel(context.Background())
	h.poster.Post(postCtx, mapSendMessageRequest(sessionID, req), resp)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	ctx := c.Context()
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		defer cancelPost()
		enc := json.NewEncoder(w)
		heartbeat := time.NewTicker(messageStreamHeartbeatInterval)
		defer heartbeat.Stop()
		for {
			select {
			case msgData, ok := <-resp:
				if !ok {
					return
				}
				payload, err := json.Marshal(msgData.Data)
				if err != nil {
					return
				}
				fmt.Fprintf(w, "event: %s\ndata: ", msgData.Type)
				if err := enc.Encode(json.RawMessage(payload)); err != nil {
					return
				}
				fmt.Fprint(w, "\n")
				w.Flush()
			case <-heartbeat.C:
				fmt.Fprint(w, ": heartbeat\n\n")
				w.Flush()
			case <-ctx.Done():
				cancelPost()
				return
			}
		}
	})

	return nil
}

// Stream handles GET /v1/sessions/:session_id/messages/stream.
func (h *MessageHandler) Stream(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")

	ch := h.sessions.Subscribe(sessionID)
	if ch == nil {
		return c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: "session not found"})
	}
	defer h.sessions.Unsubscribe(sessionID, ch)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	ctx := c.Context()
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		enc := json.NewEncoder(w)
		heartbeat := time.NewTicker(messageStreamHeartbeatInterval)
		defer heartbeat.Stop()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprint(w, "event: message\ndata: ")
				if err := enc.Encode(msg); err != nil {
					return
				}
				fmt.Fprint(w, "\n")
				w.Flush()
			case <-heartbeat.C:
				fmt.Fprint(w, ": heartbeat\n\n")
				w.Flush()
			case <-ctx.Done():
				return
			}
		}
	})

	return nil
}

// Edit handles PATCH /v1/sessions/:session_id/messages/:message_id.
func (h *MessageHandler) Edit(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotImplemented)
}
