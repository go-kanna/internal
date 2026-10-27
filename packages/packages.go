// Package packages loads the packages a generator reads, and reports what went
// wrong loading them as diagnostics.
package packages

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/go-kanna/internal/diag"
)

// Package is the go/packages type itself, so that callers need not import both.
type Package = packages.Package

// Config controls a Load.
type Config struct {
	// Dir is the directory patterns are resolved in. Empty means the working
	// directory.
	Dir string

	// Tags is the value of the -tags flag, passed to the go command as written.
	Tags string

	// Tests loads test files too. A package then comes back as its test
	// variant, which declares everything the package does plus its _test.go
	// files, and an external test package comes back as a package of its own.
	Tests bool
}

// Set is the packages one Load found, together with what went wrong loading
// them.
type Set struct {
	// Packages holds one package per import path, sorted by it.
	Packages []*Package

	// Fset is the file set every package was parsed with.
	Fset *token.FileSet

	// Diags reports what could not be loaded, in Packages or in anything they
	// import. A package with errors is still in Packages; whether to go on is
	// the caller's decision.
	Diags []diag.Diag
}

// mode is what a generator reads. NeedDeps is left out on purpose: without it
// the types of dependencies come from export data, while with it every
// dependency is parsed and type-checked from source, which more than doubles
// the load time.
const mode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedTypes |
	packages.NeedSyntax

// Load loads the packages matching patterns, which the go command matches as it
// always does; no patterns means the package in Dir. The error reports that the
// go command could not be run at all. Anything wrong with the packages
// themselves is in Set.Diags.
func Load(patterns []string, cfg Config) (Set, error) {
	dir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return Set{}, fmt.Errorf("load packages: %w", err)
	}

	fset := token.NewFileSet()
	pc := &packages.Config{Mode: mode, Dir: dir, Fset: fset, Tests: cfg.Tests}
	if cfg.Tags != "" {
		pc.BuildFlags = []string{"-tags=" + cfg.Tags}
	}

	pkgs, err := packages.Load(pc, patterns...)
	if err != nil {
		return Set{}, fmt.Errorf("load packages: %w", err)
	}
	pkgs = onePerPath(pkgs)

	return Set{Packages: pkgs, Fset: fset, Diags: diagnose(pkgs, dir)}, nil
}

// onePerPath keeps a single package per import path.
//
// With Tests, go/packages returns a package alongside its test variant, and the
// two are type-checked separately: a type read from one is not identical to the
// same type read from the other, so mixing them breaks every comparison between
// types. The test variant declares everything the package does, so it is the
// one kept. The test binary's main package declares nothing to read and is
// dropped.
func onePerPath(pkgs []*Package) []*Package {
	byPath := make(map[string]*Package, len(pkgs))
	for _, p := range pkgs {
		if p.Name == "main" && strings.HasSuffix(p.PkgPath, ".test") {
			continue
		}
		if prev, ok := byPath[p.PkgPath]; !ok || isTestVariant(p) && !isTestVariant(prev) {
			byPath[p.PkgPath] = p
		}
	}

	return slices.SortedFunc(maps.Values(byPath), func(a, b *Package) int {
		return strings.Compare(a.PkgPath, b.PkgPath)
	})
}

// isTestVariant reports whether p was compiled for a test binary, which
// go/packages shows in its ID: "example.com/p [example.com/p.test]".
func isTestVariant(p *Package) bool {
	return p.ID != p.PkgPath
}

// GeneratedFiles returns the files of pkg that carry the "Code generated ...
// DO NOT EDIT." marker, by absolute path.
func GeneratedFiles(pkg *Package) map[string]bool {
	files := make(map[string]bool)
	for _, f := range pkg.Syntax {
		if ast.IsGenerated(f) {
			files[pkg.Fset.Position(f.Pos()).Filename] = true
		}
	}
	return files
}
