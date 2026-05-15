// Package telegram provides a Telegram Bot channel adapter using long polling.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/quarkloop/runtime/pkg/channel"
	"github.com/quarkloop/runtime/pkg/message"
)

const apiBase = "https://api.telegram.org/bot"

// Config holds telegram channel configuration.
type Config struct {
	Token string
}

// Channel is the Telegram long-polling channel adapter.
type TelegramChannel struct {
	token    string
	poster   message.Poster
	ensureSn func(id, channelType, title string)
	client   *http.Client
	offset   int
	cancel   context.CancelFunc
}

// New creates a new Telegram channel.
// ensureSession is called to create a session for each incoming chat.
func New(cfg Config, p message.Poster, ensureSession func(id, channelType, title string)) *TelegramChannel {
	return &TelegramChannel{
		token:    cfg.Token,
		poster:   p,
		ensureSn: ensureSession,
		client:   &http.Client{Timeout: 35 * time.Second},
	}
}

func (c *TelegramChannel) Type() channel.ChannelType { return channel.TelegramChannelType }

// Start begins the long-polling loop.
func (c *TelegramChannel) Start(ctx context.Context) error {
	pollCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.poll(pollCtx)
	return nil
}

// Stop cancels the polling loop.
func (c *TelegramChannel) Stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func (c *TelegramChannel) poll(ctx context.Context) {
	slog.Info("telegram channel polling started", "channel", "telegram")
	for {
		select {
		case <-ctx.Done():
			slog.Info("telegram channel polling stopped", "channel", "telegram")
			return
		default:
		}

		updates, err := c.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("telegram getUpdates error", "channel", "telegram", "error", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, u := range updates {
			if u.UpdateID < c.offset {
				slog.Warn("telegram duplicate update skipped", "channel", "telegram", "update_id", u.UpdateID, "offset", c.offset)
				continue
			}
			c.handleUpdate(ctx, u)
			c.offset = u.UpdateID + 1
		}
	}
}

func (c *TelegramChannel) handleUpdate(ctx context.Context, u update) {
	incoming, ok := mapUpdate(u)
	if !ok {
		return
	}

	slog.Info("telegram message received", "channel", "telegram", "update_id", incoming.UpdateID, "chat_id", incoming.ChatID, "session_id", incoming.SessionID)
	c.ensureSn(incoming.SessionID, "telegram", incoming.Title)

	resp := make(chan message.StreamMessage, 64)
	c.poster.Post(ctx, mapIncomingPostRequest(incoming), resp)

	// Collect full response
	var sb strings.Builder
	for {
		select {
		case <-ctx.Done():
			slog.Info("telegram response collection cancelled", "channel", "telegram", "update_id", incoming.UpdateID, "chat_id", incoming.ChatID)
			return
		case msg, ok := <-resp:
			if !ok {
				if sb.Len() > 0 {
					if err := c.sendMessage(ctx, incoming.ChatID, sb.String()); err != nil {
						slog.Error("telegram sendMessage error", "channel", "telegram", "update_id", incoming.UpdateID, "chat_id", incoming.ChatID, "error", err)
					}
				}
				return
			}
			if msg.Type == "token" {
				if s, ok := msg.Data.(string); ok {
					sb.WriteString(s)
				}
			} else if msg.Type == "error" {
				if s, ok := msg.Data.(string); ok {
					sb.WriteString("\n[Agent Error: " + s + "]\n")
				}
			} else if msg.Type == "tool_start" {
				sb.WriteString("\n*(executing tool)*\n")
			}
		}
	}
}

func (c *TelegramChannel) getUpdates(ctx context.Context) ([]update, error) {
	url := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=25", apiBase, c.token, c.offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram API error: %s", result.Description)
	}
	return result.Result, nil
}

func (c *TelegramChannel) sendMessage(ctx context.Context, chatID int64, text string) error {
	url := fmt.Sprintf("%s%s/sendMessage", apiBase, c.token)

	body := fmt.Sprintf(`{"chat_id":%d,"text":%s}`, chatID, jsonString(text))
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("telegram sendMessage returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// --- Telegram API types ---

type apiResponse struct {
	OK          bool     `json:"ok"`
	Description string   `json:"description"`
	Result      []update `json:"result"`
}

type update struct {
	UpdateID int    `json:"update_id"`
	Message  *tgMsg `json:"message"`
}

type tgMsg struct {
	MessageID int    `json:"message_id"`
	Chat      tgChat `json:"chat"`
	Text      string `json:"text"`
}

type tgChat struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type incomingMessage struct {
	UpdateID  int
	ChatID    int64
	SessionID string
	Title     string
	Text      string
}

func mapUpdate(u update) (incomingMessage, bool) {
	if u.Message == nil || strings.TrimSpace(u.Message.Text) == "" {
		return incomingMessage{}, false
	}
	title := u.Message.Chat.Title
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("%s chat", u.Message.Chat.Type)
	}
	return incomingMessage{
		UpdateID:  u.UpdateID,
		ChatID:    u.Message.Chat.ID,
		SessionID: fmt.Sprintf("telegram-%d", u.Message.Chat.ID),
		Title:     title,
		Text:      u.Message.Text,
	}, true
}

func mapIncomingPostRequest(incoming incomingMessage) message.PostRequest {
	return message.PostRequest{
		SessionID: incoming.SessionID,
		Content:   incoming.Text,
	}
}
