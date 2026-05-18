//go:build e2e

package e2e

import "fmt"

func buildReleaseDryRunPrompt(workingDir string) string {
	return fmt.Sprintf(`Use Quark DevOps release automation to inspect the repository and preview the release plan for the Go project in %q.

Use version v9.9.9 and the project's build_release.json configuration. Do not publish a release or change workspace files. Reply with the repository state, project kind, planned version, and artifact names.`, workingDir)
}

func devOpsTestFailurePrompt(workingDir string) string {
	return fmt.Sprintf(`Use Quark DevOps to inspect the repository for the Go project in %q, run its tests, and explain the failure from the captured evidence.

Keep the answer concise and include the failing test name or failure line if available.`, workingDir)
}
