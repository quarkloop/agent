//go:build e2e

package e2e

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPDFPromptBuildersExposeAgentWorkflowContract(t *testing.T) {
	documents := []indexedPDFDocument{
		{Name: "Resume", Path: "/uploads/resume.pdf", Filename: "resume.pdf"},
		{Name: "Research paper", Path: "/uploads/paper.pdf", Filename: "paper.pdf"},
	}

	indexPrompt := indexPDFDocumentsPrompt(documents)
	assertPromptContains(t, indexPrompt,
		"2 files",
		"/uploads/resume.pdf",
		"/uploads/paper.pdf",
		"fs",
		"semantic extraction yourself",
		"embedding_Embed",
		"indexer_IndexDocument",
		"2 successful indexer_IndexDocument results",
		"Do not rename files",
		"write sidecars",
	)
	assertPromptExcludes(t, indexPrompt,
		`"chunkId"`,
		`"facts":`,
		`"entities":`,
		`"relations":`,
	)

	queryPrompt := indexedPDFQuestionPrompt("Which document is a resume?", len(documents))
	assertPromptContains(t, queryPrompt,
		"Do not re-read the source PDFs",
		"embedding_Embed",
		"indexer_GetContext",
		"limit 8",
		"reasoningContext only",
	)
}

func TestMarkdownPromptBuildersExposeAgentWorkflowContract(t *testing.T) {
	indexPrompt := indexMarkdownDirectoryPrompt("/uploads/company-records", 4)
	assertPromptContains(t, indexPrompt,
		"/uploads/company-records",
		"fs list",
		"fs read",
		"embedding_Embed",
		"indexer_IndexDocument",
		"4 successful indexer_IndexDocument results",
		"Do not rename files",
		"write sidecars",
	)
	assertPromptExcludes(t, indexPrompt,
		`"chunkId"`,
		`"facts":`,
		`"entities":`,
		`"relations":`,
	)

	queryPrompt := indexedMarkdownQuestionPrompt()
	assertPromptContains(t, queryPrompt,
		"Do not re-read the source files",
		"embedding_Embed",
		"indexer_GetContext",
		"limit 10",
		"reasoningContext only",
	)
}

func TestBuildReleasePromptBuilderUsesServiceFunctionContract(t *testing.T) {
	prompt := buildReleaseDryRunPrompt("/workspace/project")
	assertPromptContains(t, prompt,
		"/workspace/project",
		"Quark DevOps release automation",
		"build_release_DryRun",
		"v9.9.9",
		"build_release.json",
		"Do not use shell commands",
	)
}

func TestLongE2EPromptsAreOwnedByBuilders(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read e2e package dir: %v", err)
	}

	for _, file := range files {
		name := file.Name()
		if file.IsDir() || !strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "prompts_test.go") || name == "prompt_builders_test.go" {
			continue
		}
		assertNoLongInlinePromptLiterals(t, name)
	}
}

func assertNoLongInlinePromptLiterals(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isAgentMessageCall(call) {
			return true
		}
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquote prompt literal in %s: %v", path, err)
			}
			if len(value) > 160 {
				pos := fset.Position(lit.Pos())
				t.Fatalf("long inline prompt literal at %s; move it to a prompt builder", filepath.ToSlash(pos.String()))
			}
		}
		return true
	})
}

func isAgentMessageCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return selector.Sel.Name == "PostMessage" || selector.Sel.Name == "PostMessageTraceWithOptions"
}

func assertPromptContains(t *testing.T, prompt string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func assertPromptExcludes(t *testing.T, prompt string, rejects ...string) {
	t.Helper()
	for _, reject := range rejects {
		if strings.Contains(prompt, reject) {
			t.Fatalf("prompt contains test-built payload fragment %q:\n%s", reject, prompt)
		}
	}
}
