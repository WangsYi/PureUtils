package host

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected go.mod at %s: %v", root, err)
	}
	return root
}

func collectImports(t *testing.T, filePath string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports %s: %v", filePath, err)
	}
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		val, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		out = append(out, val)
	}
	return out
}

func walkGoFiles(t *testing.T, dir string, fn func(path string)) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") {
			fn(path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}

func TestArchitectureImportBoundaries(t *testing.T) {
	root := repoRoot(t)

	// 1. pkg/purekit/... must never import internal/* or plugins/*.
	purekitDir := filepath.Join(root, "pkg", "purekit")
	walkGoFiles(t, purekitDir, func(path string) {
		for _, imp := range collectImports(t, path) {
			if strings.HasPrefix(imp, "pureutils/internal/") || strings.HasPrefix(imp, "pureutils/plugins/") {
				t.Errorf("pkg/purekit file %s has forbidden import %q", path, imp)
			}
		}
	})

	// 2. Each plugins/<name>/... must never import internal/* or any other plugins/<other>/...
	pluginsDir := filepath.Join(root, "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		t.Fatalf("read %s: %v", pluginsDir, err)
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		pluginName := ent.Name()
		dir := filepath.Join(pluginsDir, pluginName)
		walkGoFiles(t, dir, func(path string) {
			for _, imp := range collectImports(t, path) {
				if strings.HasPrefix(imp, "pureutils/internal/") {
					t.Errorf("plugin %s file %s imports forbidden internal package %q", pluginName, path, imp)
				}
				if rest, ok := strings.CutPrefix(imp, "pureutils/plugins/"); ok {
					target, _, _ := strings.Cut(rest, "/")
					if target != pluginName {
						t.Errorf("plugin %s file %s cross-imports sibling plugin %q", pluginName, path, imp)
					}
				}
			}
		})
	}

	// 3. internal/host/*.go (including *_test.go) must never import pureutils/plugins/...
	// except files matching builtin_*.go.
	hostDir := filepath.Join(root, "internal", "host")
	walkGoFiles(t, hostDir, func(path string) {
		base := filepath.Base(path)
		allowPlugin := strings.HasPrefix(base, "builtin_") && strings.HasSuffix(base, ".go")
		for _, imp := range collectImports(t, path) {
			if strings.HasPrefix(imp, "pureutils/plugins/") && !allowPlugin {
				t.Errorf("host file %s must not directly import plugin package %q", path, imp)
			}
		}
	})

	// 4. Obsolete legacy internal/* packages must be completely removed.
	obsolete := []string{
		"app", "viewer", "archive", "zipshell", "store", "annotate", "core", "win32",
	}
	for _, name := range obsolete {
		p := filepath.Join(root, "internal", name)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("obsolete legacy directory %s still exists (err=%v)", p, err)
		}
	}
}
