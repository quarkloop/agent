//go:build e2e

package e2e

import "fmt"

func buildReleaseDryRunPrompt(workingDir string) string {
	return fmt.Sprintf(`Use Quark DevOps release automation to preview the release plan for the Go project in %q.

Call the build_release_DryRun service function with version v9.9.9 and config_path build_release.json. Do not use shell commands for this. Reply with the planned version and artifact names.`, workingDir)
}
