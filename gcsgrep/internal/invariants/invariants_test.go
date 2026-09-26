package invariants

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// moduleRoot is the gcsgrep module root, relative to this package's
// directory (go test runs with the package directory as cwd).
const moduleRoot = "../.."

// productionFiles parses every non-test .go file of the module.
func productionFiles(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) && path != moduleRoot {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		files[path] = f
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", moduleRoot, err)
	}
	if len(files) == 0 {
		t.Fatalf("no production files found under %s", moduleRoot)
	}
	return fset, files
}

// forbiddenSelectors reports every x.Name selector in production code whose
// Name is in names (and, if pkg is not empty, whose x is that identifier).
func forbiddenSelectors(t *testing.T, pkg string, names ...string) []string {
	t.Helper()
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	fset, files := productionFiles(t)
	var hits []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !set[sel.Sel.Name] {
				return true
			}
			if pkg != "" {
				id, ok := sel.X.(*ast.Ident)
				if !ok || id.Name != pkg {
					return true
				}
			}
			hits = append(hits, fset.Position(sel.Pos()).String()+": "+sel.Sel.Name)
			return true
		})
	}
	sort.Strings(hits)
	return hits
}

// VC-1.2 / FR-1.2: no code path can write object content to disk.
func TestNoDiskWrites(t *testing.T) {
	if hits := forbiddenSelectors(t, "os", "Create", "CreateTemp", "WriteFile", "OpenFile"); len(hits) > 0 {
		t.Errorf("production code writes files:\n%s", strings.Join(hits, "\n"))
	}
}

// VC-15.2 / BR-1: gcsclient.Client declares only List and Open, and no code
// uses a GCS write operation.
func TestReadOnlyGCSSurface(t *testing.T) {
	if hits := forbiddenSelectors(t, "", "NewWriter", "Delete", "Update", "Copier", "Compose", "ACL"); len(hits) > 0 {
		t.Errorf("production code uses GCS write operations:\n%s", strings.Join(hits, "\n"))
	}

	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(moduleRoot, "internal", "gcsclient", "gcsclient.go"), nil, 0)
	if err != nil {
		t.Fatalf("parsing gcsclient.go: %v", err)
	}
	var methods []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "Client" {
			return true
		}
		iface, ok := ts.Type.(*ast.InterfaceType)
		if !ok {
			t.Fatalf("gcsclient.Client is not an interface")
		}
		for _, m := range iface.Methods.List {
			for _, name := range m.Names {
				methods = append(methods, name.Name)
			}
		}
		return false
	})
	sort.Strings(methods)
	if strings.Join(methods, ",") != "List,Open" {
		t.Errorf("gcsclient.Client methods = %v, want exactly [List Open]", methods)
	}
}
