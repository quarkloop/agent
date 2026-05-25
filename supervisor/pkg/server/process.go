package server

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func stopSupervisorProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		// This primarily catches errors on Windows; on Unix, it always succeeds.
		return fmt.Errorf("finding process %d: %w", pid, err)
	}

	// 1. Send SIGTERM to ask the process to terminate.
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// If the process is already dead, sending a signal returns an error.
		// syscall.ESRCH means "no such process" on Unix systems.
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			fmt.Printf("supervisor (pid %d) is not running\n", pid)
			return nil
		}
		return fmt.Errorf("signaling process: %w", err)
	}

	// 2. Poll to wait for the process to gracefully exit.
	// We give it up to 5 seconds to shut down.
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for process %d to terminate", pid)

		case <-ticker.C:
			// Send Signal 0. This does not affect the target process, but it
			// returns an error if the process no longer exists.
			err := proc.Signal(syscall.Signal(0))
			if err != nil {
				// We received an error, which means the process is finally gone.
				fmt.Printf("supervisor (pid %d) successfully terminated\n", pid)
				return nil
			}
		}
	}
}
