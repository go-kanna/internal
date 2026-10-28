package gofile

import (
	"go/token"
	"go/types"
	"strconv"
)

// Import records the package at path, which declares itself as name, and
// returns the name the file refers to it by: name itself, or name2, name3, and
// so on when that is taken. Importing a path again returns the same name, and
// the file's own package needs no name, so it returns "".
//
// An import keeps clear of every name reserved and of every local variable
// handed out so far, in any Scope: whichever function asks for it may be the
// one that declares the variable.
func (f *File) Import(path, name string) string {
	if path == f.pkgPath {
		return ""
	}
	if spec, ok := f.imports[path]; ok {
		return spec.local
	}

	local := f.free(name, f.locals)
	f.imports[path] = importSpec{name: name, local: local}
	f.file[local] = true
	return local
}

// Qualifier is a types.Qualifier that imports each package it is asked about,
// so that rendering a type through types.TypeString both names it and records
// what the file has to import.
func (f *File) Qualifier(p *types.Package) string {
	if p == nil {
		return ""
	}
	return f.Import(p.Path(), p.Name())
}

// Reserve marks names that the file must not introduce, such as what the
// package already declares at the top level and what the body declares there
// itself. Reserve them before asking for any other name.
func (f *File) Reserve(names ...string) {
	for _, n := range names {
		f.file[n] = true
	}
}

// Scope hands out the local variables of one function in the body.
type Scope struct {
	f     *File
	names map[string]bool
}

// Scope starts the names of another function. Functions keep their locals
// apart from the file's names, not from each other's.
func (f *File) Scope() Scope {
	return Scope{f: f, names: make(map[string]bool)}
}

// Name returns base, or base2, base3, and so on, whichever the function has not
// used and no import or reserved name has taken.
func (s Scope) Name(base string) string {
	n := s.f.free(base, s.names)
	s.names[n] = true
	s.f.locals[n] = true
	return n
}

// free returns base or the first of base2, base3, ... that neither the file's
// scope nor also holds.
func (f *File) free(base string, also map[string]bool) string {
	candidate := base
	for n := 2; f.taken(candidate) || also[candidate]; n++ {
		candidate = base + strconv.Itoa(n)
	}
	return candidate
}

// taken reports whether name is in the file's scope, or is a keyword or a
// predeclared identifier, which no file may introduce.
func (f *File) taken(name string) bool {
	return f.file[name] || token.IsKeyword(name) || types.Universe.Lookup(name) != nil
}
