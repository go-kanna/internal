// Package pkgtest builds loaded packages for tests, without putting a module on
// disk.
package pkgtest

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"testing"

	"github.com/go-kanna/internal/packages"
)

// Load type-checks files as one package and returns it the way packages.Load
// would, with its name, syntax, and types. Its package clause names it, and the
// name doubles as its import path. Files are parsed in name order, so that
// positions come out the same from run to run. Anything that does not
// type-check fails the test.
func Load(t testing.TB, files map[string]string) *packages.Package {
	t.Helper()

	fset := token.NewFileSet()
	names := slices.Sorted(maps.Keys(files))
	syntax := make([]*ast.File, 0, len(names))
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, files[name], parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		syntax = append(syntax, f)
	}
	if len(syntax) == 0 {
		t.Fatal("pkgtest.Load: no files")
	}

	name := syntax[0].Name.Name
	pkg, err := (&types.Config{Importer: importer.Default()}).Check(name, fset, syntax, nil)
	if err != nil {
		t.Fatalf("type-check %s: %v", name, err)
	}

	return &packages.Package{
		ID:              name,
		Name:            name,
		PkgPath:         name,
		GoFiles:         names,
		CompiledGoFiles: names,
		Fset:            fset,
		Syntax:          syntax,
		Types:           pkg,
	}
}
