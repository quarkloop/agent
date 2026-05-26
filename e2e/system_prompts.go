//go:build e2e

package e2e

func systemReadOnlyInspectionPrompt() string {
	return `Use Quark System to inspect this machine without changing anything.

Summarize the operating system, kernel, uptime, current load and memory metrics, disk usage for the root filesystem, a small process overview, and listening ports or network state if available. Keep the answer operational and mention anything you could not inspect safely.`
}
