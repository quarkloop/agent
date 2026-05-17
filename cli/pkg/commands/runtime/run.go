package runtimecmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/quarkloop/cli/pkg/util"
	spacemodel "github.com/quarkloop/pkg/space"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

var flags struct {
	port    int
	timeout time.Duration
}

// NewRunCommand returns the "run" command.
func NewRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [dir]",
		Short: "Start the runtime for the current space via the supervisor",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runRun,
	}

	cmd.Flags().IntVar(&flags.port, "port", 0, "Port for the runtime HTTP API (0 = supervisor picks)")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Second, "Maximum time to wait for the agent to become ready")
	return cmd
}

func runRun(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	workingDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	if _, err := os.Stat(workingDir); err != nil {
		return fmt.Errorf("working dir: %w", err)
	}

	data, err := spacemodel.ReadQuarkfile(workingDir)
	if err != nil {
		return err
	}

	space, err := spacemodel.NameFromQuarkfile(data)
	if err != nil {
		return err
	}

	sup := supclient.New()
	if _, err := sup.GetSpace(cmd.Context(), space); err != nil {
		if !supclient.IsNotFound(err) {
			return fmt.Errorf("lookup space: %w", err)
		}
		// Space doesn't exist yet.
		return fmt.Errorf("space %s not found", space)
	}

	rt, err := sup.StartRuntime(cmd.Context(), space, flags.port)
	if err != nil {
		return fmt.Errorf("start runtime: %w", err)
	}

	if err := waitUntilRunning(cmd.Context(), sup, rt.ID, flags.timeout); err != nil {
		return fmt.Errorf("runtime failed to become ready: %w", err)
	}

	util.Successf("Runtime started (space=%s, pid=%d, port=%d)", rt.Space, rt.PID, rt.Port)
	fmt.Printf("  Runtime ID: %s\n", rt.ID)
	fmt.Printf("  URL:        %s\n", rt.URL())
	fmt.Println("\nUse 'quark activity query --follow' to stream activity.")
	fmt.Println("Use 'quark stop' to stop.")
	return nil
}

// waitUntilRunning polls the supervisor until the runtime reports running.
func waitUntilRunning(ctx context.Context, sup *supclient.Client, runtimeID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rt, err := sup.GetRuntime(ctx, runtimeID)
		if err == nil && rt.Status == supclient.RuntimeRunning {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}
