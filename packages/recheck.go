package packages

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/output"
)

// Recheck type-checks the root packages of set again, in memory, as they are
// once changes are applied: a file written takes the place of the one at its
// path, or joins the package of its directory that its package clause names,
// and a file removed leaves its package. It runs no go command. A run can thus
// tell, before it writes anything, whether the code calling what it writes
// compiles, which includes what go/types could not check before: the rest of
// an expression whose operand is undefined.
//
// Every root is checked again, as one the run writes nothing into may call one
// it does, in import order, each importing those checked before it. A test
// variant is checked a second time without its tests, as go build compiles it
// and as the other packages import it. Anything else is imported as the load
// has it.
//
// It reports the errors of the files written by hand, sorted by position, each
// once, and none of a package importing one with errors, which go build does
// not compile either. A generated file is left to its generator: a file of
// changes may import what the load did not read, and any other is written
// again by its own generator in the same go generate. A file of changes that
// does not parse is reported alone.
//
// A package that is not a root but imports one keeps the types of the root as
// loaded, which differ from those checked again, and an error may come of it.
// It takes patterns that leave out a package between two roots, or an external
// test importing a package that imports the one it tests.
func Recheck(set Set, changes []output.Change) []diag.Diag {
	outputs := make(map[string]*ast.File, len(changes)) // nil for a file removed
	var ds []diag.Diag
	for _, c := range changes {
		if c.Remove {
			outputs[c.Path] = nil
			continue
		}
		f, err := parser.ParseFile(set.Fset, c.Path, c.Data, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			pos, msg := token.Position{Filename: c.Path}, err.Error()
			if list, ok := errors.AsType[scanner.ErrorList](err); ok {
				pos, msg = list[0].Pos, list[0].Msg
			}
			ds = append(ds, diag.Errorf(pos, "the code generated does not parse: %s", msg))
		}
		outputs[c.Path] = f
	}
	if len(ds) > 0 {
		return ds
	}

	r := rechecker{
		set:     set,
		units:   make(map[string]*unit),
		checked: make(map[string]*types.Package),
		broken:  make(map[string]bool),
		skip:    make(map[string]bool),
	}
	for path := range outputs {
		r.skip[path] = true
	}
	for _, p := range set.Packages {
		r.add(p, p.ID, isTestVariant(p), outputs)
		if isTestVariant(p) {
			r.add(p, p.PkgPath, false, outputs)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(r.units)) {
		r.check(id)
	}

	slices.SortFunc(r.ds, func(a, b diag.Diag) int {
		return cmp.Or(
			strings.Compare(a.Pos.Filename, b.Pos.Filename),
			cmp.Compare(a.Pos.Line, b.Pos.Line),
			cmp.Compare(a.Pos.Column, b.Pos.Column),
			strings.Compare(a.Message, b.Message),
		)
	})
	return slices.CompactFunc(r.ds, func(a, b diag.Diag) bool { return a.Pos == b.Pos && a.Message == b.Message })
}

// unit is a package Recheck checks again: a root, or a test variant built
// without its tests.
type unit struct {
	pkg   *Package
	files []*ast.File
}

type rechecker struct {
	set     Set
	units   map[string]*unit          // by the ID Package.Imports knows them by
	checked map[string]*types.Package // by the same
	broken  map[string]bool           // whether a unit or one it imports has errors
	skip    map[string]bool           // the generated files, whose errors go unreported
	loaded  map[string]*types.Package // what the roots import, by path; built when first needed
	ds      []diag.Diag
}

// add adds the unit of pkg under id, with outputs in place and with its tests or
// not, unless that leaves it no file, as it does an external test package.
func (r *rechecker) add(pkg *Package, id string, tests bool, outputs map[string]*ast.File) {
	var (
		files []*ast.File
		seen  = make(map[string]bool)
	)
	for _, f := range pkg.Syntax {
		path := pkg.Fset.Position(f.Pos()).Filename
		if !tests && strings.HasSuffix(path, "_test.go") {
			continue
		}
		seen[path] = true
		if out, ok := outputs[path]; ok {
			f = out
		}
		if f == nil {
			continue
		}
		if ast.IsGenerated(f) {
			r.skip[path] = true
		}
		files = append(files, f)
	}

	var dir string
	if len(pkg.GoFiles) > 0 {
		dir = filepath.Dir(pkg.GoFiles[0])
	}
	for _, path := range slices.Sorted(maps.Keys(outputs)) {
		f := outputs[path]
		if f == nil || seen[path] || filepath.Dir(path) != dir || f.Name.Name != pkg.Name ||
			!tests && strings.HasSuffix(path, "_test.go") {
			continue
		}
		files = append(files, f)
	}

	if len(files) > 0 {
		r.units[id] = &unit{pkg: pkg, files: files}
	}
}

// check checks the unit under id, after the units it imports.
func (r *rechecker) check(id string) {
	if _, done := r.checked[id]; done {
		return
	}
	u := r.units[id]
	r.checked[id] = nil // under way; imports do not cycle

	var broken bool
	for _, f := range u.files {
		for _, spec := range f.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if dep := r.resolve(u, path); r.units[dep] != nil {
				r.check(dep)
				broken = broken || r.broken[dep]
			}
		}
	}

	var ds []diag.Diag
	conf := types.Config{
		Importer: importerFunc(func(path string) (*types.Package, error) { return r.importFor(u, path) }),
		Sizes:    u.pkg.TypesSizes,
		Error: func(err error) {
			e, ok := err.(types.Error) //nolint:errorlint // types.Config.Error is given types.Error alone
			if !ok {
				return
			}
			if pos := e.Fset.Position(e.Pos); !r.skip[pos.Filename] {
				ds = append(ds, diag.Errorf(pos, "%s", e.Msg))
			}
		},
	}
	if m := u.pkg.Module; m != nil && m.GoVersion != "" {
		conf.GoVersion = "go" + m.GoVersion
	}
	p, _ := conf.Check(u.pkg.PkgPath, r.set.Fset, u.files, nil) // the errors went to conf.Error

	r.checked[id] = p
	r.broken[id] = broken || len(ds) > 0
	if !broken {
		r.ds = append(r.ds, ds...)
	}
}

// resolve returns the ID of what u imports as path: the test variant an
// external test imports, or else the package itself, which a file of changes
// may be the only one to import.
func (r *rechecker) resolve(u *unit, path string) string {
	if imp := u.pkg.Imports[path]; imp != nil {
		return imp.ID
	}
	return path
}

// importFor imports path for u: as checked again, or else as the load has it,
// looking through all of it, as a file of changes may import what no other file
// of u does.
func (r *rechecker) importFor(u *unit, path string) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	if p := r.checked[r.resolve(u, path)]; p != nil {
		return p, nil
	}
	if u.pkg.Types != nil {
		for _, p := range u.pkg.Types.Imports() {
			if p.Path() == path {
				return p, nil
			}
		}
	}
	if r.loaded == nil {
		r.loaded = imports(r.set.Packages)
	}
	if p := r.loaded[path]; p != nil {
		return p, nil
	}
	return nil, fmt.Errorf("%s is not loaded", path)
}

// imports returns what pkgs import, directly or not, by path.
func imports(pkgs []*Package) map[string]*types.Package {
	loaded := make(map[string]*types.Package)
	var next []*types.Package
	for _, p := range pkgs {
		if p.Types != nil {
			next = append(next, p.Types.Imports()...)
		}
	}
	for len(next) > 0 {
		var deeper []*types.Package
		for _, p := range next {
			if _, ok := loaded[p.Path()]; !ok {
				loaded[p.Path()] = p
				deeper = append(deeper, p.Imports()...)
			}
		}
		next = deeper
	}
	return loaded
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }
