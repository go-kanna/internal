// Package scan reads the types a package declares: its structs, with their
// fields, or every type of any kind.
//
// It applies no filter beyond skipping generated files, because the generators
// disagree on what matters: some take the structs a directive marks, others
// every struct in a package but those excluded. A filter here would bend the
// model toward one of them. Even generated files are read for a generator that
// asks, by AllStructs.
package scan

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"

	"github.com/go-kanna/internal/packages"
)

// Struct is a struct type declared at the top level of a package.
type Struct struct {
	PkgPath string
	PkgName string
	Name    string

	// Named is the declared type, whose underlying type is a *types.Struct.
	// Named.TypeParams tells a generic declaration.
	Named *types.Named

	// Pos is where the type's name is declared.
	Pos token.Position

	// Doc is the comment on the declaration, or nil. A comment on a
	// parenthesized group of declarations documents the group rather than each
	// type in it, so it stands in only for a group of one.
	Doc *ast.CommentGroup

	// Fields holds every field in declaration order, unexported, embedded, and
	// blank ones included.
	Fields []Field
}

// Type is a type declared at the top level of a package, of any kind: a
// struct, a named string, an interface.
type Type struct {
	PkgPath string
	PkgName string
	Name    string

	// Named is the declared type. Named.Underlying tells its kind, and
	// Named.TypeParams a generic declaration.
	Named *types.Named

	// Pos is where the type's name is declared.
	Pos token.Position

	// Doc is the comment on the declaration, or nil, as for a Struct.
	Doc *ast.CommentGroup
}

// Field is one field of a Struct.
type Field struct {
	// Name is the field's name, which for an embedded field is the name of the
	// type embedded.
	Name string

	Type types.Type

	// Tag is the field's struct tag, in which each generator looks up its own
	// key.
	Tag reflect.StructTag

	Pos      token.Position
	Exported bool
	Embedded bool
}

// Types returns the types declared in pkgs, structs among them, in the order
// they are declared, skipping what Structs skips: generated files and type
// aliases.
func Types(pkgs []*packages.Package) []Type {
	decls := declared(pkgs, false)
	found := make([]Type, 0, len(decls))
	for _, d := range decls {
		found = append(found, d.Type)
	}
	return found
}

// Structs returns the struct types declared in pkgs, in the order they are
// declared: package by package, file by file. A file carrying the
// generated-code marker is skipped, so that a generator never reads its own
// output, and so is a type alias, which names a type declared elsewhere.
func Structs(pkgs []*packages.Package) []Struct {
	return structs(pkgs, false)
}

// AllStructs is Structs reading generated files too, for a generator that
// writes into a package other than the one it reads, and so never meets its own
// output there. Its input may well be generated itself, such as the models sqlc
// writes.
func AllStructs(pkgs []*packages.Package) []Struct {
	return structs(pkgs, true)
}

func structs(pkgs []*packages.Package, generated bool) []Struct {
	var structs []Struct
	for _, d := range declared(pkgs, generated) {
		if s, ok := structOf(d); ok {
			structs = append(structs, s)
		}
	}
	return structs
}

// decl is a Type with the package declaring it.
type decl struct {
	Type

	pkg *packages.Package
}

// declared returns the types declared in pkgs, package by package, file by
// file, but for type aliases, reading generated files only if generated is
// set.
func declared(pkgs []*packages.Package, generated bool) []decl {
	var decls []decl
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			if !generated && ast.IsGenerated(file) {
				continue
			}
			for _, d := range file.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					if t, ok := typeOf(pkg, gd, spec); ok {
						decls = append(decls, decl{Type: t, pkg: pkg})
					}
				}
			}
		}
	}
	return decls
}

// typeOf reads spec, one of the specs of gd, as a Type, reporting false when it
// declares an alias.
func typeOf(pkg *packages.Package, gd *ast.GenDecl, spec ast.Spec) (Type, bool) {
	ts, ok := spec.(*ast.TypeSpec)
	if !ok || ts.Assign.IsValid() {
		return Type{}, false
	}
	// The blank type _ is in no scope.
	obj, ok := pkg.Types.Scope().Lookup(ts.Name.Name).(*types.TypeName)
	if !ok {
		return Type{}, false
	}
	named, ok := obj.Type().(*types.Named) // what every type declaration but an alias declares
	if !ok {
		return Type{}, false
	}

	doc := ts.Doc
	if doc == nil && len(gd.Specs) == 1 {
		doc = gd.Doc
	}
	return Type{
		PkgPath: pkg.PkgPath,
		PkgName: pkg.Name,
		Name:    obj.Name(),
		Named:   named,
		Pos:     pkg.Fset.Position(obj.Pos()),
		Doc:     doc,
	}, true
}

// structOf reads d as a Struct, reporting false when it declares anything else.
func structOf(d decl) (Struct, bool) {
	st, ok := d.Named.Underlying().(*types.Struct)
	if !ok {
		return Struct{}, false
	}

	fields := make([]Field, st.NumFields())
	for i := range st.NumFields() {
		v := st.Field(i)
		fields[i] = Field{
			Name:     v.Name(),
			Type:     v.Type(),
			Tag:      reflect.StructTag(st.Tag(i)),
			Pos:      d.pkg.Fset.Position(v.Pos()),
			Exported: v.Exported(),
			Embedded: v.Embedded(),
		}
	}

	return Struct{
		PkgPath: d.PkgPath,
		PkgName: d.PkgName,
		Name:    d.Name,
		Named:   d.Named,
		Pos:     d.Pos,
		Doc:     d.Doc,
		Fields:  fields,
	}, true
}
