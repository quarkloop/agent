//go:build e2e

package e2e

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

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
