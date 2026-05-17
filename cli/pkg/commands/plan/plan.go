// Package plancmd provides CLI commands for managing execution plans.
// All operations are HTTP calls against the running agent.
package plancmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/runtimedial"
)

func NewPlanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Manage execution plans",
	}
	cmd.AddCommand(newPlanGetCmd())
	cmd.AddCommand(newPlanListCmd())
	cmd.AddCommand(newPlanApproveCmd())
	cmd.AddCommand(newPlanRejectCmd())
	return cmd
}

func newPlanGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get the agent's current plan",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := runtimedial.Current(cmd.Context())
			if err != nil {
				return err
			}
			p, err := c.Plan(cmd.Context())
			if err != nil {
				return fmt.Errorf("get plan: %w", err)
			}
			data, _ := json.MarshalIndent(p, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
}

func newPlanListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the agent's current plan",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := runtimedial.Current(cmd.Context())
			if err != nil {
				return err
			}
			p, err := c.Plan(cmd.Context())
			if err != nil {
				return fmt.Errorf("list plans: %w", err)
			}
			fmt.Printf("%-10s  %s\n", p.Status, p.Goal)
			return nil
		},
	}
}

func newPlanApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve [plan-id]",
		Short: "Approve the current active plan",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := runtimedial.Current(cmd.Context())
			if err != nil {
				return err
			}
			planID := ""
			if len(args) > 0 {
				planID = args[0]
			}
			if _, err := c.ApprovePlan(cmd.Context(), planID); err != nil {
				return fmt.Errorf("approve plan: %w", err)
			}
			fmt.Println("Plan approved")
			return nil
		},
	}
}

func newPlanRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reject [plan-id]",
		Short: "Reject the current active plan",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := runtimedial.Current(cmd.Context())
			if err != nil {
				return err
			}
			planID := ""
			if len(args) > 0 {
				planID = args[0]
			}
			if err := c.RejectPlan(cmd.Context(), planID); err != nil {
				return fmt.Errorf("reject plan: %w", err)
			}
			fmt.Println("Plan rejected")
			return nil
		},
	}
}
