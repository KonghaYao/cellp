package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

const (
	registryImportPath = "github.com/cellp/cellp/internal/registry"
	serveImportPath    = "github.com/cellp/cellp/internal/serve"
)

func TestMainDispatchesAgentHelp(t *testing.T) {
	if cmdAgent([]string{"--help"}) != 0 {
		t.Fatal("expected help exit 0")
	}
}

func TestMainUnknownAgentArg(t *testing.T) {
	if cmdAgent([]string{"--bogus"}) != 2 {
		t.Fatal("expected usage exit 2")
	}
}

func TestCmdAgentEntryDoesNotImportServeOrRegistry(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "agent.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	imports := map[string]string{}
	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		if spec.Name != nil {
			switch spec.Name.Name {
			case "_":
				continue
			case ".":
				if path == registryImportPath || path == serveImportPath {
					t.Fatalf("agent.go must not dot-import %s", path)
				}
				continue
			default:
				imports[spec.Name.Name] = path
			}
			continue
		}
		if i := strings.LastIndex(path, "/"); i >= 0 {
			imports[path[i+1:]] = path
		}
	}
	for local, path := range imports {
		if path == registryImportPath || path == serveImportPath {
			t.Fatalf("agent.go must not import %s as %q", path, local)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		path, known := imports[x.Name]
		if !known {
			return true
		}
		if path == registryImportPath && sel.Sel.Name == "Open" {
			t.Fatalf("agent.go must not call %s.Open", x.Name)
		}
		if path == serveImportPath && sel.Sel.Name == "Run" {
			t.Fatalf("agent.go must not call %s.Run", x.Name)
		}
		return true
	})
	if !strings.Contains(mustReadAgentGo(), "agentrun") {
		t.Fatal("cellp agent must enter through agentrun")
	}
}

func mustReadAgentGo() string {
	data, err := os.ReadFile("agent.go")
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestFinishAgentRunPureCancellationExitZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.Join(context.Canceled, ctx.Err())
	if code := finishAgentRun(ctx, err); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestFinishAgentRunJoinedReleaseFailureNonZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.Join(context.Canceled, errors.New("agent lease release: nodereg unavailable"))
	if code := finishAgentRun(ctx, err); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestFinishAgentRunJoinedShutdownErrorNonZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.Join(context.Canceled, errors.New("agent server shutdown: closed"))
	if code := finishAgentRun(ctx, err); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}
