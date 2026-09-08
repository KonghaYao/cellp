package agentrun

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	registryImportPath = "github.com/cellp/cellp/internal/registry"
	serveImportPath    = "github.com/cellp/cellp/internal/serve"
)

func collectProductionGoFiles(t *testing.T) []string {
	t.Helper()
	var prodFiles []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		prodFiles = append(prodFiles, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(prodFiles) == 0 {
		t.Fatal("expected production Go sources under internal/agentrun")
	}
	return prodFiles
}

func importPath(spec *ast.ImportSpec) string {
	return strings.Trim(spec.Path.Value, `"`)
}

func localNameForImport(spec *ast.ImportSpec) (string, bool) {
	if spec.Name != nil {
		switch spec.Name.Name {
		case "_":
			return "", false
		case ".":
			return ".", true
		default:
			return spec.Name.Name, true
		}
	}
	path := importPath(spec)
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:], true
	}
	return path, true
}

func fileImportSpecs(path string) ([]*ast.ImportSpec, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	return f.Imports, nil
}

func localImportMap(path string) (map[string]string, error) {
	specs, err := fileImportSpecs(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, spec := range specs {
		name, ok := localNameForImport(spec)
		if !ok {
			continue
		}
		out[name] = importPath(spec)
	}
	return out, nil
}

func TestAgentRunProductionSourcesDirectImportsExcludeRegistryAndServe(t *testing.T) {
	for _, name := range collectProductionGoFiles(t) {
		specs, err := fileImportSpecs(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range specs {
			path := importPath(spec)
			if path == registryImportPath || path == serveImportPath {
				t.Fatalf("%s must not directly import %s (agent path uses nodereg + registry relay)", name, path)
			}
		}
	}
}

func TestAgentRunProductionASTNoRegistryOpenOrServeRunSelectors(t *testing.T) {
	fset := token.NewFileSet()
	for _, name := range collectProductionGoFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		imports, err := localImportMap(name)
		if err != nil {
			t.Fatal(err)
		}
		if imports["."] == registryImportPath || imports["."] == serveImportPath {
			t.Fatalf("%s must not dot-import %s", name, imports["."])
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
				t.Fatalf("%s must not call registry.Open (resolved import alias %q)", name, x.Name)
			}
			if path == serveImportPath && sel.Sel.Name == "Run" {
				t.Fatalf("%s must not call serve.Run (resolved import alias %q)", name, x.Name)
			}
			return true
		})
	}
}

func TestAgentRunDepsHasNoLocalRegistryStoreFactory(t *testing.T) {
	specs, err := fileImportSpecs("deps.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range specs {
		if importPath(spec) == registryImportPath {
			t.Fatal("Deps must not import registry; standalone agent uses agentstore over registry relay")
		}
	}
	data, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "agentstore.NewStore") {
		t.Fatal("run.go must wire agentstore over registry relay transport")
	}
}

func TestRunExitOutcomePureCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.Join(context.Canceled, ctx.Err())
	code, logLine := RunExitOutcome(ctx, err)
	if code != 0 || logLine != "" {
		t.Fatalf("code=%d log=%q", code, logLine)
	}
}

func TestRunExitOutcomeJoinedShutdownErrorNonZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.Join(context.Canceled, errors.New("agent server shutdown: listener closed"))
	code, logLine := RunExitOutcome(ctx, err)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if logLine == "" || strings.Contains(logLine, "=") {
		t.Fatalf("unexpected log summary: %q", logLine)
	}
}

func TestRunExitOutcomeReleaseFailureNonZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.Join(context.Canceled, errors.New("agent lease release: nodereg unavailable"))
	code, logLine := RunExitOutcome(ctx, err)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if logLine == "" {
		t.Fatal("expected log summary")
	}
}

func TestRunExitOutcomeStartupFailureNonZero(t *testing.T) {
	err := errors.New("nodereg activate: denied")
	code, logLine := RunExitOutcome(context.Background(), err)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if logLine == "" {
		t.Fatal("expected log summary")
	}
}

func TestRunExitOutcomeRedactsSecretLikeLogLine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.Join(context.Canceled, errors.New("SUPER_SECRET=hunter2"))
	code, logLine := RunExitOutcome(ctx, err)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if strings.Contains(logLine, "SECRET") {
		t.Fatalf("log leaked secret material: %q", logLine)
	}
	if logLine == "" {
		t.Fatal("expected log summary")
	}
}

func TestSafeRunErrorSummaryRedactsEnvLikeValues(t *testing.T) {
	s := SafeRunErrorSummary(errors.New("SUPER_SECRET=hunter2"))
	if strings.Contains(s, "hunter2") {
		t.Fatalf("summary must not echo secret: %q", s)
	}
}
