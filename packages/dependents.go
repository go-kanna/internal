package packages

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/go-kanna/internal/diag"
)

// Dependents returns the import paths of the packages of the module in dir that
// import pkgPath, directly or through one another, their tests included,
// sorted. An external test package is named by the package it tests, which a
// load with Config.Tests brings along. It lists imports alone, type-checking
// nothing, for a generator whose output is called mostly from other packages,
// as kanna-i18n's is, to load only those.
func Dependents(dir, pkgPath string) ([]string, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedImports,
		Dir:   dir,
		Tests: true,
	}, "./...")
	if err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}

	importers := make(map[string][]string)
	for _, p := range pkgs {
		if p.Name == "main" && strings.HasSuffix(p.PkgPath, ".test") {
			continue
		}
		path := p.PkgPath
		if isTestVariant(p) && strings.HasSuffix(p.Name, "_test") {
			path = strings.TrimSuffix(path, "_test")
		}
		for imported := range p.Imports {
			importers[imported] = append(importers[imported], path)
		}
	}

	found := make(map[string]bool)
	for next := []string{pkgPath}; len(next) > 0; {
		var deeper []string
		for _, path := range next {
			for _, importer := range importers[path] {
				if importer != pkgPath && !found[importer] {
					found[importer] = true
					deeper = append(deeper, importer)
				}
			}
		}
		next = deeper
	}
	return slices.Sorted(maps.Keys(found)), nil
}

// Introduced returns those of ds that the load of s did not report already:
// what the changes given to Recheck cause, or uncover behind a call they make
// resolve. A generator whose output is called from all over the module, as
// kanna-i18n's is, stops on these alone, and not on an error some code had
// before the run.
func (s Set) Introduced(ds []diag.Diag) []diag.Diag {
	// The load knows a position by its line and column alone, without the
	// offset Recheck has.
	type key struct {
		file         string
		line, column int
		msg          string
	}
	keyOf := func(d diag.Diag) key { return key{d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Message} }
	had := make(map[key]bool, len(s.Diags))
	for _, d := range s.Diags {
		had[keyOf(d)] = true
	}
	return slices.DeleteFunc(slices.Clone(ds), func(d diag.Diag) bool { return had[keyOf(d)] })
}
