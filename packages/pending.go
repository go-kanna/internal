package packages

import (
	"go/ast"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-kanna/internal/diag"
)

var (
	// callForms are the type errors about a function, which they name as the
	// source does.
	callForms = []*regexp.Regexp{
		regexp.MustCompile(`^undefined: ([\w.]+)$`),
		regexp.MustCompile(`^(?:not enough|too many) arguments in call to ([\w.]+)$`),
		regexp.MustCompile(`^assignment mismatch: .* but ([\w.]+) returns `),
		regexp.MustCompile(` in argument to ([\w.]+)$`),
	}

	// methodForms are the type errors about a method, which they name by a
	// type as go/types writes it, and the method's name.
	methodForms = []*regexp.Regexp{
		regexp.MustCompile(`\(type \*?(\S+) has no field or method (\w+)`),
		regexp.MustCompile(`: \*?(\S+) does not (?:implement|satisfy) .*\((?:missing|wrong type for) method (\w+)\)`),
	}

	unusedImport = regexp.MustCompile(`^("[^"]+") imported and not used$`)
)

// Pending returns whether an error of a root of s says only that one of
// names is missing, or is used other than as declared, as an error does before
// a run writes or changes what it names. A name is a function,
// "<import path>.<name>", or a method, "<import path>.<type>.<method>".
//
// The error is told by its message: an undefined function, a call to one with
// the wrong arguments or results, a type that has no such method or misses one
// an interface asks for, or a dot import used by nothing but calls to such
// functions. A generator that stops before Recheck can tell leaves those out of
// the errors it reports along with its own: they say nothing of what is wrong.
func (s Set) Pending(names map[string]bool) func(diag.Diag) bool {
	files := make(map[string]source)
	for _, p := range s.Packages {
		for _, f := range p.Syntax {
			files[p.Fset.Position(f.Pos()).Filename] = source{pkg: p, file: f}
		}
	}
	return func(d diag.Diag) bool {
		src, ok := files[d.Pos.Filename]
		if !ok || d.Severity != diag.Error {
			return false
		}
		first, _, _ := strings.Cut(d.Message, "\n")
		for _, re := range callForms {
			if m := re.FindStringSubmatch(first); m != nil {
				for _, name := range src.funcs(m[1]) {
					if names[name] {
						return true
					}
				}
			}
		}
		for _, re := range methodForms {
			if m := re.FindStringSubmatch(first); m != nil {
				if t := src.typeName(m[1]); t != "" && names[t+"."+m[2]] {
					return true
				}
			}
		}
		if m := unusedImport.FindStringSubmatch(first); m != nil {
			if path, _ := strconv.Unquote(m[1]); src.imports(path, ".") {
				for name := range names {
					if strings.HasPrefix(name, path+".") {
						return true
					}
				}
			}
		}
		return false
	}
}

// source is a file of a root, with its package.
type source struct {
	pkg  *Package
	file *ast.File
}

// funcs returns the full names a function may have that the file names as
// expr, as the source writes it: one of its package or of a package it dot
// imports, or one of a package it imports under the qualifier.
func (src source) funcs(expr string) []string {
	q, name, qualified := strings.Cut(expr, ".")
	if !qualified {
		names := []string{src.pkg.PkgPath + "." + expr}
		for _, spec := range src.file.Imports {
			if path, err := strconv.Unquote(spec.Path.Value); err == nil && src.imports(path, ".") {
				names = append(names, path+"."+expr)
			}
		}
		return names
	}
	var names []string
	for _, spec := range src.file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err == nil && src.imports(path, q) {
			names = append(names, path+"."+name)
		}
	}
	return names
}

// imports reports whether the file imports path as local, a name or ".".
func (src source) imports(path, local string) bool {
	for _, spec := range src.file.Imports {
		if p, err := strconv.Unquote(spec.Path.Value); err != nil || p != path {
			continue
		}
		name := src.name(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == local {
			return true
		}
	}
	return false
}

// name returns the name of the package at path that the package of the file
// imports.
func (src source) name(path string) string {
	if src.pkg.Types != nil {
		for _, p := range src.pkg.Types.Imports() {
			if p.Path() == path {
				return p.Name()
			}
		}
	}
	return path[strings.LastIndex(path, "/")+1:]
}

// typeName returns the full name of the type that go/types writes as t for the
// package of the file: unqualified for one of the package, and qualified by
// the name of another package, or by its quoted path when two share a name.
func (src source) typeName(t string) string {
	i := strings.LastIndex(t, ".")
	if i < 0 {
		return src.pkg.PkgPath + "." + t
	}
	q, name := t[:i], t[i+1:]
	if path, err := strconv.Unquote(q); err == nil {
		return path + "." + name
	}
	if src.pkg.Types != nil {
		for _, p := range src.pkg.Types.Imports() {
			if p.Name() == q {
				return p.Path() + "." + name
			}
		}
	}
	return ""
}
