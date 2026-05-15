// Package activitycmd provides CLI commands for the agent activity log.
// All operations are HTTP calls against the running agent.
package activitycmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/agentdial"
	agentclient "github.com/quarkloop/runtime/pkg/client"
)

func NewActivityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activity",
		Short: "Manage the activity log",
	}
	cmd.AddCommand(newActivityQueryCmd())
	return cmd
}

func newActivityQueryCmd() *cobra.Command {
	var eventType string
	var limit int
	var follow bool
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query activity log entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := agentdial.Current(cmd.Context())
			if err != nil {
				return err
			}

			if follow {
				fmt.Println("Streaming live activity... (Ctrl+C to stop)")
				return c.StreamActivity(cmd.Context(), func(record agentclient.ActivityRecord) {
					if eventType != "" && record.Type != eventType {
						return
					}
					fmt.Printf("%s  %-30s  %s  %s\n", record.Timestamp, record.Type, record.SessionID, record.Data)
				})
			}

			records, err := c.Activity(cmd.Context(), limit)
			if err != nil {
				return fmt.Errorf("query activity: %w", err)
			}
			if len(records) == 0 {
				fmt.Println("No activity.")
				return nil
			}
			for _, rec := range records {
				if eventType != "" && rec.Type != eventType {
					continue
				}
				fmt.Printf("%s  %-30s  %s  %s\n",
					rec.Timestamp.Format(time.RFC3339), rec.Type, rec.SessionID, rec.Data)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&eventType, "type", "", "Filter by event type")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of entries")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream live activity")
	return cmd
}
