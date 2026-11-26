package host

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func hasHanRune(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func extractPrintfVerbs(s string) []string {
	var verbs []string
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		for j < len(s) {
			c := s[j]
			if strings.ContainsRune("bcdeEfFgGoOpqstTxUvXw", rune(c)) {
				verbs = append(verbs, s[i:j+1])
				break
			}
			if !strings.ContainsRune("+- #0123456789.*[]", rune(c)) {
				break
			}
			j++
		}
		i = j
	}
	return verbs
}

func selectorCallName(expr ast.Expr) (pkgOrRecv, fn string) {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		fn = e.Sel.Name
		switch x := e.X.(type) {
		case *ast.Ident:
			pkgOrRecv = x.Name
		case *ast.CallExpr:
			_, rfn := selectorCallName(x.Fun)
			pkgOrRecv = rfn
		case *ast.SelectorExpr:
			pkgOrRecv = x.Sel.Name
		}
	case *ast.Ident:
		fn = e.Name
	}
	return pkgOrRecv, fn
}

func unquoteLiteral(lit *ast.BasicLit) (string, bool) {
	if lit == nil || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

func isLoggerCall(pkgOrRecv, fn string) bool {
	rLower := strings.ToLower(pkgOrRecv)
	fLower := strings.ToLower(fn)
	if strings.Contains(rLower, "log") {
		return true
	}
	switch fLower {
	case "logf", "logln", "log", "infof", "info", "warnf", "warn", "errorf", "debugf", "debug":
		return strings.Contains(rLower, "host") || strings.Contains(rLower, "cb") || strings.Contains(rLower, "c") || pkgOrRecv == ""
	}
	return false
}

func TestI18nAudit(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	roots := []string{
		filepath.Join(repoRoot, "main.go"),
		filepath.Join(repoRoot, "pkg", "purekit"),
		filepath.Join(repoRoot, "internal", "host"),
		filepath.Join(repoRoot, "plugins"),
	}

	var goFiles []string
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			t.Fatalf("stat %s: %v", root, err)
		}
		if !info.IsDir() {
			goFiles = append(goFiles, root)
			continue
		}
		err = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				goFiles = append(goFiles, p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	fset := token.NewFileSet()
	for _, path := range goFiles {
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			rel = path
		}
		relSlash := filepath.ToSlash(rel)

		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", relSlash, err)
		}

		// Catalog file defines registered ZH->EN pairs intentionally.
		isPureImageI18n := relSlash == "plugins/pureimage/i18n.go"

		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return false
			}
			stack = append(stack, n)

			switch node := n.(type) {
			case *ast.CallExpr:
				pkg, fn := selectorCallName(node.Fun)
				if pkg == "i18n" && fn == "F" && len(node.Args) >= 2 {
					zhLit, ok1 := node.Args[0].(*ast.BasicLit)
					enLit, ok2 := node.Args[1].(*ast.BasicLit)
					if ok1 && ok2 {
						zh, _ := unquoteLiteral(zhLit)
						en, _ := unquoteLiteral(enLit)
						zv := extractPrintfVerbs(zh)
						ev := extractPrintfVerbs(en)
						if !reflect.DeepEqual(zv, ev) {
							pos := fset.Position(node.Pos())
							t.Errorf("%s:%d: i18n.F format verb mismatch: zh=%q (%v) vs en=%q (%v)", relSlash, pos.Line, zh, zv, en, ev)
						}
					}
				}
				if pkg == "fmt" && (fn == "Errorf" || fn == "Sprintf") && len(node.Args) >= 1 {
					if inner, ok := node.Args[0].(*ast.CallExpr); ok {
						ipkg, ifn := selectorCallName(inner.Fun)
						if ipkg == "i18n" && ifn == "S" {
							pos := fset.Position(node.Pos())
							t.Errorf("%s:%d: fmt.%s called with non-constant i18n.S format string", relSlash, pos.Line, fn)
						}
					}
				}

			case *ast.BasicLit:
				if node.Kind != token.STRING {
					return true
				}
				val, ok := unquoteLiteral(node)
				if !ok || !hasHanRune(val) {
					return true
				}
				if val == "简体中文" || val == "中文" {
					return true
				}
				if isPureImageI18n {
					return true
				}

				if isAllowedHanContext(relSlash, stack) {
					return true
				}

				pos := fset.Position(node.Pos())
				t.Errorf("%s:%d: raw untranslated Han string literal %q", relSlash, pos.Line, val)
			}
			return true
		})
	}
}

func isAllowedHanContext(relSlash string, stack []ast.Node) bool {
	// stack[len(stack)-1] is the BasicLit itself.
	for i := len(stack) - 2; i >= 0; i-- {
		parent := stack[i]
		child := stack[i+1]

		switch p := parent.(type) {
		case *ast.CallExpr:
			pkg, fn := selectorCallName(p.Fun)
			// i18n.S(zh, en), i18n.F(zhFmt, enFmt, ...), i18n.RegisterCatalog(...)
			if pkg == "i18n" && (fn == "S" || fn == "F") && len(p.Args) > 0 && p.Args[0] == child {
				return true
			}
			if pkg == "i18n" && fn == "RegisterCatalog" {
				return true
			}
			// tr("...") catalog keys in pureimage
			if pkg == "" && (fn == "tr" || fn == "trf") && len(p.Args) > 0 && p.Args[0] == child {
				return true
			}
			// Archive extension localized label table in purezip/zip_util.go: add(cat, zh, en, ...)
			if relSlash == "plugins/purezip/zip_util.go" && pkg == "" && fn == "add" && len(p.Args) >= 3 && p.Args[1] == child {
				return true
			}
			if isLoggerCall(pkg, fn) {
				return true
			}

		case *ast.CompositeLit:
			if ident, ok := p.Type.(*ast.Ident); ok {
				// localizedErr{"zh", "en"} or zipLabelPair{"zh", "en"}
				if (ident.Name == "localizedErr" || ident.Name == "zipLabelPair") && len(p.Elts) >= 2 && p.Elts[0] == child {
					return true
				}
			}

		case *ast.ValueSpec:
			for _, name := range p.Names {
				if name.Name == "zipEncodingPairs" {
					// First field of zipLabelPair inside []zipLabelPair{...}
					if cl, ok := stack[len(stack)-2].(*ast.CompositeLit); ok && len(cl.Elts) >= 2 && cl.Elts[0] == stack[len(stack)-1] {
						return true
					}
				}
			}

		case *ast.KeyValueExpr:
			if k, ok := p.Key.(*ast.Ident); ok {
				if k.Name == "Keywords" {
					return true
				}
				if k.Name == "zh" {
					return true
				}
			}

		case *ast.CaseClause:
			// Switch cases matching legacy/ZH identifiers or labels (e.g. parseZipJobOp, UpdateSourceIDFromLabel).
			for _, expr := range p.List {
				if expr == child {
					return true
				}
			}

		case *ast.FuncDecl:
			if p.Name != nil {
				switch p.Name.Name {
				case "downloadSourcePair", "UpdateSourceLabel", "UpdateSourceIDFromLabel", "zipEncodingPairs":
					return true
				}
			}

		case *ast.RangeStmt:
			if ident, ok := p.Value.(*ast.Ident); ok && ident.Name == "kw" {
				return true
			}
		}
	}
	return false
}
