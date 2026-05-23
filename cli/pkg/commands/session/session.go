// Package sessioncmd provides CLI commands for managing supervisor-owned
// sessions. Sessions live in the supervisor — the agent is notified of
// create/delete events through supervisor-owned NATS subjects.
package sessioncmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	spacemodel "github.com/quarkloop/pkg/space"
)

// currentSpace returns the space name from the Quarkfile in the current
// working directory. Session commands operate on this space.
func currentSpace() (string, error) {
	return spacemodel.CurrentName()
}

func NewSessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage sessions",
	}
	cmd.AddCommand(newSessionCreateCmd())
	cmd.AddCommand(newSessionGetCmd())
	cmd.AddCommand(newSessionDeleteCmd())
	cmd.AddCommand(newSessionListCmd())
	return cmd
}

func newSessionCreateCmd() *cobra.Command {
	var sessType, title string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			space, err := currentSpace()
			if err != nil {
				return err
			}
			c, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			sess, err := c.CreateSession(cmd.Context(), clientcontract.CreateSessionRequest{
				SpaceID: space,
				Type:    clientcontract.SessionType(sessType),
				Title:   title,
			})
			if err != nil {
				return fmt.Errorf("create session: %w", err)
			}
			fmt.Printf("Session created: %s\n", sess.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessType, "type", "chat", "Session type (chat|subagent|cron)")
	cmd.Flags().StringVar(&title, "title", "", "Session title")
	return cmd
}

func newSessionGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <session-id>",
		Short: "Get a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			space, err := currentSpace()
			if err != nil {
				return err
			}
			c, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			sess, err := c.GetSession(cmd.Context(), space, args[0])
			if err != nil {
				return fmt.Errorf("get session: %w", err)
			}
			data, _ := json.MarshalIndent(sess, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
}

func newSessionDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <session-id>",
		Short: "Delete a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			space, err := currentSpace()
			if err != nil {
				return err
			}
			c, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.DeleteSession(cmd.Context(), space, args[0]); err != nil {
				return fmt.Errorf("delete session: %w", err)
			}
			fmt.Printf("Session deleted: %s\n", args[0])
			return nil
		},
	}
}

func newSessionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sessions for the current space",
		RunE: func(cmd *cobra.Command, _ []string) error {
			space, err := currentSpace()
			if err != nil {
				return err
			}
			c, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			sessions, err := c.ListSessions(cmd.Context(), space)
			if err != nil {
				return fmt.Errorf("list sessions: %w", err)
			}
			if len(sessions.Sessions) == 0 {
				fmt.Println("No sessions.")
				return nil
			}
			for _, s := range sessions.Sessions {
				fmt.Printf("%-10s %s  %s\n", s.Type, s.ID, s.Title)
			}
			return nil
		},
	}
}
