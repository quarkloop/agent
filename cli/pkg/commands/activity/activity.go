// Package activitycmd provides CLI commands for the agent activity log.
package activitycmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/runtimeconnect"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
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
			conn, err := runtimeconnect.CurrentSpaceClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Client.Close()

			if follow {
				fmt.Println("Streaming live activity... (Ctrl+C to stop)")
				records, errs, stop, err := conn.Client.SubscribeRuntimeActivity(cmd.Context())
				if err != nil {
					return err
				}
				defer stop()
				return streamActivity(cmd.Context(), records, errs, eventType)
			}

			records, err := conn.Client.RuntimeActivity(cmd.Context(), conn.SpaceID, limit)
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
				printActivityRecord(rec)
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&eventType, "type", "", "Filter by event type")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of entries")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream live activity")
	return cmd
}

func streamActivity(ctx context.Context, records <-chan clientcontract.RuntimeActivityRecord, errs <-chan error, eventType string) error {
	for {
		select {
		case record := <-records:
			if eventType != "" && record.Type != eventType {
				continue
			}
			printActivityRecord(record)
		case err := <-errs:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func printActivityRecord(record clientcontract.RuntimeActivityRecord) {
	fmt.Printf("%s  %-30s  %s  %s\n", record.Timestamp.Format(time.RFC3339), record.Type, record.SessionID, record.Data)
}
