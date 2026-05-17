//go:build e2e

package e2e

import "fmt"

func buildReleaseDryRunPrompt(workingDir string) string {
	return fmt.Sprintf(`Use Quark DevOps release automation to preview the release plan for the Go project in %q.

Use version v9.9.9 and the project's build_release.json configuration. Do not publish a release or change workspace files. Reply with the planned version and artifact names.`, workingDir)
}
