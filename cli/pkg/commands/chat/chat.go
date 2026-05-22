// Package chatcmd provides the user-facing chat command.
package chatcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	spacemodel "github.com/quarkloop/pkg/space"
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

			space, err := spacemodel.CurrentName()
			if err != nil {
				return err
			}
			control, err := natsclient.ConnectFromEnv(ctx)
			if err != nil {
				return err
			}
			defer control.Close()
			targetSession := sessionID
			if createSession {
				created, err := createChatSession(ctx, control, space, title)
				if err != nil {
					return err
				}
				targetSession = created.ID
				fmt.Fprintf(cmd.ErrOrStderr(), "Session: %s\n", targetSession)
			}
			if targetSession == "" {
				return fmt.Errorf("session is required; pass --session <id> or --new")
			}
			credential, err := control.IssueSessionCredential(ctx, space, targetSession)
			if err != nil {
				return fmt.Errorf("issue session credential: %w", err)
			}
			sessionClient, err := natsclient.ConnectWithCredential(ctx, credential)
			if err != nil {
				return err
			}
			defer sessionClient.Close()

			stdout := cmd.OutOrStdout()
			stderr := cmd.ErrOrStderr()
			events, errs, stop, err := sessionClient.SubscribeSessionEvents(ctx, targetSession)
			if err != nil {
				return err
			}
			defer stop()
			if _, err := sessionClient.SendSessionMessage(ctx, clientcontract.SendMessageRequest{
				SpaceID:   space,
				SessionID: targetSession,
				Content:   content,
			}); err != nil {
				return err
			}
			if err := streamSessionEvents(ctx, stdout, stderr, events, errs, showTools); err != nil {
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

func createChatSession(ctx context.Context, control *natsclient.Client, space, title string) (clientcontract.SessionInfo, error) {
	session, err := control.CreateSession(ctx, clientcontract.CreateSessionRequest{
		SpaceID: space,
		Type:    clientcontract.SessionTypeChat,
		Title:   title,
	})
	if err != nil {
		return clientcontract.SessionInfo{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func streamSessionEvents(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
	events <-chan clientcontract.SessionEvent,
	errs <-chan error,
	showTools bool,
) error {
	for {
		select {
		case event := <-events:
			if event.Type == "done" {
				return nil
			}
			if err := printEvent(stdout, stderr, event, showTools); err != nil {
				return err
			}
		case err := <-errs:
			return err
		case <-ctx.Done():
			return fmt.Errorf("session stream ended before completion: %w", ctx.Err())
		}
	}
}

func printEvent(stdout, stderr io.Writer, event clientcontract.SessionEvent, showTools bool) error {
	switch event.Type {
	case "text", "token":
		var token string
		if err := json.Unmarshal(event.Payload, &token); err != nil {
			return fmt.Errorf("decode token event: %w", err)
		}
		_, err := fmt.Fprint(stdout, token)
		return err
	case "tool_start":
		if showTools {
			fmt.Fprintf(stderr, "tool start: %s\n", eventToolName(event.Payload))
		}
	case "tool_result":
		if showTools {
			fmt.Fprintf(stderr, "tool result: %s\n", eventToolName(event.Payload))
		}
	case "error":
		var message string
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			var payload struct {
				Message  string `json:"message"`
				Boundary string `json:"boundary"`
				Category string `json:"category"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err == nil && payload.Message != "" {
				message = payload.Message
				if payload.Category != "" {
					message = fmt.Sprintf("%s [%s/%s]", message, payload.Boundary, payload.Category)
				}
			} else {
				message = strings.TrimSpace(string(event.Payload))
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
