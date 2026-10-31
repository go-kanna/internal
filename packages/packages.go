// Package packages loads the packages a generator reads, reports what went
// wrong loading them as diagnostics, and type-checks them again as they are
// once what the generator writes is in place.
package packages

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/output"
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

	// TypesInfo records in Package.TypesInfo what the identifiers and
	// expressions of the syntax denote, for a generator that reads an
	// expression, such as a function named in a list, and not only
	// declarations. It covers the packages matched, not what they import.
	TypesInfo bool

	// Deps reads what the packages import from source as well, rather than
	// from export data. Export data holds of a package only what its own API
	// needs of the packages it imports, so a package the ones matched reach
	// only through another comes without its functions, which a generator
	// looking for constructors there needs. It about doubles the load time.
	Deps bool

	// Regenerated names the files the caller is about to write again in the
	// packages it loads, such as "validate_gen.go". What they hold is about to
	// be replaced, so one that no longer type-checks is set aside for the load;
	// see Load. Of the files not named here, only a generated one that still
	// declares a package since renamed is ever set aside, as it cannot compile
	// there whichever generator wrote it. A hand-written file is never set
	// aside, even under a name given here: it is not the caller's to replace.
	Regenerated []string

	// Overlay names files by absolute path, each read as holding what is given
	// here and not what is on disk, for a caller who wants to read packages as
	// they would be with those files changed. What the disk holds stays.
	Overlay map[string][]byte
}

// Set is the packages one Load found, together with what went wrong loading
// them.
type Set struct {
	// Packages holds the packages the patterns match, the roots of the load,
	// one per import path, sorted by it. What they import is loaded too, but
	// is not among them.
	Packages []*Package

	// Fset is the file set every package was parsed with.
	Fset *token.FileSet

	// Diags reports what could not be loaded, in Packages or in anything they
	// import. A package with errors is still in Packages; whether to go on is
	// the caller's decision.
	Diags []diag.Diag

	// SetAside lists, by absolute path, the generated files whose
	// declarations had to be dropped for the packages to load. Code calling into one fails until it
	// is regenerated, which the caller may want to point out alongside Diags.
	SetAside []string
}

// mode is what every generator reads; Config.TypesInfo and Config.Deps add to
// it. The module and the sizes, which the load has anyway, let Recheck
// type-check as the load did.
const mode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedTypes |
	packages.NeedSyntax |
	packages.NeedModule |
	packages.NeedTypesSizes

// Load loads the packages matching patterns, which the go command matches as it
// always does; no patterns means the package in Dir. The error reports that the
// go command could not be run at all. Anything wrong with the packages
// themselves is in Set.Diags.
//
// A change to a declaration can leave the output an earlier run generated from
// it referring to what is gone, and a package that does not type-check cannot be
// read to generate that output again. So when an error points into one of the
// files in cfg.Regenerated, it is set aside for the load, without touching the
// disk, in two steps: first only its function bodies are replaced, which is
// where stale output almost always breaks and which keeps hand-written callers
// compiling; if errors still point into it, the whole file is emptied. Errors
// anywhere else never set anything aside, so a real mistake is reported as it
// is.
//
// Renaming a package leaves the output of an earlier run declaring the old
// name, and the go command then refuses the whole package, as likely blaming a
// hand-written file as the generated one. So before anything else, a generated
// file that declares another package than the hand-written files of its
// package, which all declare the same one, is emptied, declaring theirs,
// whether it is in cfg.Regenerated or not. It cannot compile there anyway, and
// the generator that wrote it sets aside the others' output and writes its own
// again on its run, so go generate ./... gets through a rename.
func Load(patterns []string, cfg Config) (Set, error) {
	set, err := load(patterns, cfg, nil)
	if err != nil {
		return set, err
	}
	return setAside(patterns, cfg, set)
}

// load is a single Load, with overlay standing in for the named files.
func load(patterns []string, cfg Config, overlay map[string][]byte) (Set, error) {
	dir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return Set{}, fmt.Errorf("load packages: %w", err)
	}

	m := mode
	if cfg.TypesInfo {
		m |= packages.NeedTypesInfo
	}
	if cfg.Deps {
		m |= packages.NeedDeps
	}
	merged := maps.Clone(cfg.Overlay)
	if merged == nil {
		merged = overlay
	} else {
		maps.Copy(merged, overlay)
	}

	fset := token.NewFileSet()
	pc := &packages.Config{Mode: m, Dir: dir, Fset: fset, Tests: cfg.Tests, Overlay: merged}
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

// Paths returns the import paths of the packages of s, sorted. An external test
// package goes with the package it tests, which nothing can import it as.
func (s Set) Paths() []string {
	paths := make([]string, 0, len(s.Packages))
	for _, p := range s.Packages {
		if !isTestVariant(p) || !strings.HasSuffix(p.Name, "_test") {
			paths = append(paths, p.PkgPath)
		}
	}
	return paths
}

// GeneratedFiles returns the files of pkg that carry the "Code generated ...
// DO NOT EDIT." marker, by absolute path.
//
// A file that does not parse as far as its package clause, as one whose marker
// a merge left in conflict, is read again and told as output.IsGenerated tells
// it, for its generator to write it again.
func GeneratedFiles(pkg *Package) map[string]bool {
	files := make(map[string]bool)
	for _, f := range pkg.Syntax {
		tf := pkg.Fset.File(f.FileStart)
		if tf == nil {
			continue
		}
		switch path := tf.Name(); {
		case ast.IsGenerated(f):
			files[path] = true
		case f.Package == token.NoPos:
			if src, err := os.ReadFile(path); err == nil && output.IsGenerated(src) { //nolint:gosec // a file of the load
				files[path] = true
			}
		}
	}
	return files
}
