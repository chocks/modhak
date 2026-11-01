package advisor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestPure enforces AGENTS.md: the advisor has no HTTP, database/sql, store,
// environment or clock access. Time is passed in via Input.Now.
func TestPure(t *testing.T) {
	forbiddenImports := []string{"net/http", "database/sql", "os", "github.com/chocks/modhak/internal/store", "github.com/go-sql-driver/mysql"}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenImports {
				if path == bad {
					t.Errorf("%s imports %s", e.Name(), path)
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since" || sel.Sel.Name == "Until") {
					t.Errorf("%s calls time.%s; pass time in Input.Now", e.Name(), sel.Sel.Name)
				}
			}
			return true
		})
	}
}
