// Package contextcmd provides read-only Harness context inspection commands.
package contextcmd

import (
	"fmt"
	"io"

	"github.com/quarkloop/cli/pkg/runtimeconnect"
	"github.com/quarkloop/cli/pkg/spacecontext"
	harnessv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/harness/v1"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

func NewContextCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "context", Short: "Inspect model context reports"}
	cmd.AddCommand(newReportCommand(), newReportsCommand())
	return cmd
}

func newReportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "report <report-id>",
		Short: "Retrieve one Harness context report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			connection, err := connect(cmd)
			if err != nil {
				return err
			}
			defer connection.Client.Close()
			report, err := connection.Client.GetContextReport(cmd.Context(), connection.SpaceID, args[0])
			if err != nil {
				return fmt.Errorf("get context report: %w", err)
			}
			return writeReport(cmd.OutOrStdout(), report)
		},
	}
}

func newReportsCommand() *cobra.Command {
	var limit int32
	cmd := &cobra.Command{
		Use:   "reports <session-id>",
		Short: "List Harness context reports for one session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			connection, err := connect(cmd)
			if err != nil {
				return err
			}
			defer connection.Client.Close()
			reports, err := connection.Client.ContextReports(cmd.Context(), connection.SpaceID, args[0], limit)
			if err != nil {
				return fmt.Errorf("list context reports: %w", err)
			}
			for _, report := range reports {
				if err := writeReport(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 50, "Maximum reports to return")
	return cmd
}

func connect(cmd *cobra.Command) (runtimeconnect.SpaceClient, error) {
	space, err := spacecontext.FromCommand(cmd)
	if err != nil {
		return runtimeconnect.SpaceClient{}, err
	}
	return runtimeconnect.SpaceRuntimeClient(cmd.Context(), space)
}

func writeReport(out io.Writer, report *harnessv1.ContextReport) error {
	data, err := protojson.MarshalOptions{Indent: "  "}.Marshal(report)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
