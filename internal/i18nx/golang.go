package i18nx

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cerrConstructors are the `cerr.*` helpers whose first argument is a message
// template. `New` is deliberately absent: its first argument is the error type.
var cerrConstructors = map[string]bool{
	"Validation": true, "Permission": true, "NotFound": true, "LinkExists": true,
	"Timestamp": true, "Duplicate": true, "Auth": true, "Internal": true, "Mandatory": true,
	"TooMany": true, "Unavailable": true,
}

// goSkip lists what stays out of the catalogue on purpose. These strings do
// become English, they just never pass through `_()`:
//
//   - internal/mcp — an MCP tool description is read by a model, not by a
//     person looking at an interface.
//   - cmd — the CLI is English.
//   - meta.Registry.Validate — a developer error surfaced by `migrate`.
//
// The list lives here, next to the collector, so `--check` and the reader see
// the same set of exclusions.
var goSkip = []string{
	"internal/mcp/",
	"cmd/",
	"internal/meta/meta.go#Registry.Validate",
}

func skipGo(file, fn string) bool {
	file = filepath.ToSlash(file)
	for _, s := range goSkip {
		path, name, scoped := strings.Cut(s, "#")
		if !strings.HasPrefix(file, path) {
			continue
		}
		if !scoped || name == fn {
			return true
		}
	}
	return false
}

// CollectGo walks dir for .go files and collects the first string literal of
// every `cerr.<Constructor>(…)` and `.T(…)` call. Paths in the Set are
// relative to rel.
func CollectGo(s *Set, dir, rel string) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == "testdata" || strings.HasPrefix(n, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		name := p
		if r, err := filepath.Rel(rel, p); err == nil {
			name = r
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return nil // a file that does not parse is the compiler's problem
		}
		collectGoFile(s, fset, f, filepath.ToSlash(name))
		return nil
	})
}

func collectGoFile(s *Set, fset *token.FileSet, f *ast.File, name string) {
	// enclosing tracks the function a call sits in, so an exclusion can be
	// scoped to one function instead of a whole file.
	var enclosing string
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncDecl:
			enclosing = funcName(v)
		case *ast.CallExpr:
			sel, ok := v.Fun.(*ast.SelectorExpr)
			if !ok || len(v.Args) == 0 {
				return true
			}
			// `cerr.X("…")` and `.WithTitleKey("…")` are templates the
			// border translates; `.T("…")` is translated on the spot.
			isCerr := isCerrCall(sel) || sel.Sel.Name == "WithTitleKey"
			if !isCerr && sel.Sel.Name != "T" {
				return true
			}
			if skipGo(name, enclosing) {
				return true
			}
			// The key is the first argument of `cerr.X` and of `Ctx.T`, but the
			// *second* of `I18n.T(lang, key, …)`, which takes the language
			// first. Without looking there, every string translated for a
			// reader who is not the requester — the body of a recovery e-mail —
			// would stay out of the catalogue, and `--check` would report
			// nothing missing while the mail went out in English.
			lit, ok := v.Args[0].(*ast.BasicLit)
			if (!ok || lit.Kind != token.STRING) && !isCerr && len(v.Args) > 1 {
				lit, ok = v.Args[1].(*ast.BasicLit)
			}
			if !ok || lit.Kind != token.STRING {
				// `c.T(f.Label)` is the normal shape: a label translated at
				// the point of construction, whose key the metadata
				// collector already has. Only a computed *template* is worth
				// a word, because the border cannot translate it.
				if isCerr {
					s.AddDynamic(name, fset.Position(v.Pos()).Line)
				}
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			s.Add(text, name, fset.Position(lit.Pos()).Line)
		}
		return true
	})
}

func isCerrCall(sel *ast.SelectorExpr) bool {
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "cerr" && cerrConstructors[sel.Sel.Name]
}

func funcName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	t := d.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + d.Name.Name
	}
	return d.Name.Name
}
