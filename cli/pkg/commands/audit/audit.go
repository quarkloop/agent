// Package auditcmd provides redacted audit retrieval commands backed by the
// supervisor-owned audit control API.
package auditcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/natsclient"
	"github.com/quarkloop/cli/pkg/spacecontext"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func NewAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Inspect redacted service-call audit records",
	}
	cmd.AddCommand(newGetCommand(), newListCommand(), newRetentionCommand())
	return cmd
}

func newGetCommand() *cobra.Command {
	var space string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "get <reference-id>",
		Short: "Retrieve one service-call record by reference ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedSpace, err := resolveSpace(cmd, space)
			if err != nil {
				return err
			}
			client, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer client.Close()
			record, err := client.GetAuditRecord(cmd.Context(), resolvedSpace, args[0])
			if err != nil {
				return fmt.Errorf("get audit record: %w", err)
			}
			return writeAuditRecord(cmd.OutOrStdout(), record, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&space, "space", "", "Space ID; defaults to the current space")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output redacted record metadata as JSON")
	return cmd
}

func newListCommand() *cobra.Command {
	var request clientcontract.AuditListRequest
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent service-call audit records",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			space, err := resolveSpace(cmd, request.SpaceID)
			if err != nil {
				return err
			}
			request.SpaceID = space
			client, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer client.Close()
			page, err := client.ListAuditRecords(cmd.Context(), request)
			if err != nil {
				return fmt.Errorf("list audit records: %w", err)
			}
			return writeAuditPage(cmd.OutOrStdout(), page, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&request.SpaceID, "space", "", "Space ID; defaults to the current space")
	cmd.Flags().StringVar(&request.SessionID, "session", "", "Filter by session ID")
	cmd.Flags().StringVar(&request.RunID, "run", "", "Filter by run ID")
	cmd.Flags().StringVar(&request.Service, "service", "", "Filter by service owner")
	cmd.Flags().StringVar(&request.Function, "function", "", "Filter by service function")
	cmd.Flags().IntVar(&request.Limit, "limit", 50, "Maximum records to return")
	cmd.Flags().Uint64Var(&request.Cursor, "cursor", 0, "Continue after the returned sequence cursor")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output redacted record metadata as JSON")
	return cmd
}

func newRetentionCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "retention",
		Short: "Show service-call audit retention limits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := natsclient.ConnectFromEnv(cmd.Context())
			if err != nil {
				return err
			}
			defer client.Close()
			retention, err := client.AuditRetention(cmd.Context())
			if err != nil {
				return fmt.Errorf("get audit retention: %w", err)
			}
			return writeRetention(cmd.OutOrStdout(), retention, jsonOutput)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output retention limits as JSON")
	return cmd
}

func resolveSpace(cmd *cobra.Command, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	return spacecontext.FromCommand(cmd)
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
