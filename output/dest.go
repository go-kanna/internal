package output

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PackageName returns the package of a file named generatedFile, generated into
// dir. Every Go file in a directory has to agree on its package, so the one the
// files already there declare wins; failing that, want, when given; failing
// that, the name of dir. A want that disagrees with what is declared is an
// error.
//
// The files counted are the ones the build would compile. A //go:build ignore
// helper declaring package main does not count, and neither does
// generatedFile, so that what an earlier run wrote cannot pin the name of the
// next.
func PackageName(dir, generatedFile, want string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("package name for %s: %w", dir, err)
	}
	declared, err := declaredPackage(abs, generatedFile)
	if err != nil {
		return "", err
	}

	name := filepath.Base(abs)
	switch {
	case declared != "" && want != "" && want != declared:
		return "", fmt.Errorf("%s already holds package %s, not %s", dir, declared, want)
	case declared != "":
		name = declared
	case want != "":
		name = want
	}
	if !token.IsIdentifier(name) {
		return "", fmt.Errorf("%q is not a valid package name", name)
	}
	return name, nil
}

// declaredPackage returns the package the files in dir that the build would
// compile declare, leaving out generatedFile, or "" when there are none.
func declaredPackage(dir, generatedFile string) (string, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}

	ctx := build.Default
	ctx.ReadDir = func(d string) ([]fs.FileInfo, error) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", d, err)
		}
		infos := make([]fs.FileInfo, 0, len(entries))
		for _, e := range entries {
			if e.Name() == generatedFile {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", d, err)
			}
			infos = append(infos, info)
		}
		return infos, nil
	}

	pkg, err := ctx.ImportDir(dir, 0)
	if _, ok := errors.AsType[*build.NoGoError](err); ok {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("package name for %s: %w", dir, err)
	}
	return pkg.Name, nil
}

// DeclaredNames returns the top-level names that package pkgName declares in
// dir, each with where it is declared, so that a generator can keep clear of
// them. generatedFile is left out, since it is about to be replaced.
//
// Every file is read, whatever its build constraints: a file generated for all
// platforms must not redeclare a name that exists only on some. Files of
// another package, such as an external test package, share the directory but
// not the names, and are skipped.
func DeclaredNames(dir, generatedFile, pkgName string) map[string]token.Position {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	fset := token.NewFileSet()
	names := make(map[string]token.Position)
	for _, e := range entries {
		if e.IsDir() || e.Name() == generatedFile || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.SkipObjectResolution)
		if err != nil || f.Name.Name != pkgName {
			continue
		}
		for _, decl := range f.Decls {
			for _, id := range declared(decl) {
				if id.Name != "_" {
					names[id.Name] = fset.Position(id.Pos())
				}
			}
		}
	}
	return names
}

// declared returns the names decl brings into the package scope. A method
// belongs to its receiver's type instead.
func declared(decl ast.Decl) []*ast.Ident {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			return []*ast.Ident{d.Name}
		}
	case *ast.GenDecl:
		var ids []*ast.Ident
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				ids = append(ids, s.Name)
			case *ast.ValueSpec:
				ids = append(ids, s.Names...)
			}
		}
		return ids
	}
	return nil
}

// SameDir reports whether a and b name one existing directory, through
// symlinks and, on a case-insensitive file system, different spellings. A
// generator writing into the package it reads would make the file import its
// own package.
func SameDir(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}
