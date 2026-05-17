// Package chatcmd provides the user-facing chat command.
package chatcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/runtimedial"
	spacemodel "github.com/quarkloop/pkg/space"
	rtclient "github.com/quarkloop/runtime/pkg/client"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

// NewChatCommand returns the "chat" command.
func NewChatCommand() *cobra.Command {
	var sessionID string
	var createSession bool
	var title string
	var timeout time.Duration
	var showTools bool

	cmd := &cobra.Command{
		Use:   "chat [message]",
		Short: "Send a message to the running runtime",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content := strings.TrimSpace(strings.Join(args, " "))
			if content == "" {
				return fmt.Errorf("message cannot be empty")
			}

			ctx := cmd.Context()
			var cancel context.CancelFunc
			if timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, timeout)
			} else {
				ctx, cancel = context.WithCancel(ctx)
			}
			defer cancel()

			rtClient, _, err := runtimedial.CurrentWithTransportOptions(ctx, rtclient.WithHTTPClient(&http.Client{}))
			if err != nil {
				return err
			}

			targetSession := sessionID
			if createSession {
				created, err := createChatSession(ctx, title)
				if err != nil {
					return err
				}
				targetSession = created.ID
				fmt.Fprintf(cmd.ErrOrStderr(), "Session: %s\n", targetSession)
			}
			if targetSession == "" {
				return fmt.Errorf("session is required; pass --session <id> or --new")
			}
			if err := waitForRuntimeSession(ctx, rtClient, targetSession); err != nil {
				return err
			}

			stdout := cmd.OutOrStdout()
			stderr := cmd.ErrOrStderr()
			err = rtClient.PostSessionMessage(ctx, targetSession, content, func(event rtclient.SSEEvent) error {
				return printEvent(stdout, stderr, event, showTools)
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout)
			return nil
		},
	}

	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session id to send the message to")
	cmd.Flags().BoolVar(&createSession, "new", false, "Create a new chat session before sending")
	cmd.Flags().StringVar(&title, "title", "", "Title for --new chat sessions")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Maximum time to wait for the streamed response")
	cmd.Flags().BoolVar(&showTools, "show-tools", true, "Print tool progress to stderr")
	return cmd
}

func createChatSession(ctx context.Context, title string) (supclient.Session, error) {
	space, err := spacemodel.CurrentName()
	if err != nil {
		return supclient.Session{}, err
	}
	session, err := supclient.New().CreateSession(ctx, space, supclient.CreateSessionRequest{
		Type:  supclient.SessionTypeChat,
		Title: title,
	})
	if err != nil {
		return supclient.Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func waitForRuntimeSession(ctx context.Context, rtClient *rtclient.Client, sessionID string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		_, err := rtClient.ListSessionMessages(ctx, sessionID)
		if err == nil {
			return nil
		}
		if !rtclient.IsNotFound(err) {
			return fmt.Errorf("lookup runtime session: %w", err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("runtime did not mirror session %q: %w", sessionID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func printEvent(stdout, stderr io.Writer, event rtclient.SSEEvent, showTools bool) error {
	switch event.Type {
	case "text", "token":
		var token string
		if err := json.Unmarshal(event.Data, &token); err != nil {
			return fmt.Errorf("decode token event: %w", err)
		}
		_, err := fmt.Fprint(stdout, token)
		return err
	case "tool_start":
		if showTools {
			fmt.Fprintf(stderr, "tool start: %s\n", eventToolName(event.Data))
		}
	case "tool_result":
		if showTools {
			fmt.Fprintf(stderr, "tool result: %s\n", eventToolName(event.Data))
		}
	case "error":
		var message string
		if err := json.Unmarshal(event.Data, &message); err != nil {
			var payload struct {
				Message  string `json:"message"`
				Boundary string `json:"boundary"`
				Category string `json:"category"`
			}
			if err := json.Unmarshal(event.Data, &payload); err == nil && payload.Message != "" {
				message = payload.Message
				if payload.Category != "" {
					message = fmt.Sprintf("%s [%s/%s]", message, payload.Boundary, payload.Category)
				}
			} else {
				message = strings.TrimSpace(string(event.Data))
			}
		}
		return fmt.Errorf("agent error: %s", message)
	}
	return nil
}

func eventToolName(data []byte) string {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Name == "" {
		return "(unknown)"
	}
	return payload.Name
}
