// Package scan reads the struct types a package declares.
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
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			if !generated && ast.IsGenerated(file) {
				continue
			}
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					if s, ok := structOf(pkg, gd, spec); ok {
						structs = append(structs, s)
					}
				}
			}
		}
	}
	return structs
}

// structOf reads spec, one of the specs of gd, as a Struct, reporting false when
// it declares anything else.
func structOf(pkg *packages.Package, gd *ast.GenDecl, spec ast.Spec) (Struct, bool) {
	ts, ok := spec.(*ast.TypeSpec)
	if !ok || ts.Assign.IsValid() {
		return Struct{}, false
	}
	// The blank type _ is in no scope.
	obj, ok := pkg.Types.Scope().Lookup(ts.Name.Name).(*types.TypeName)
	if !ok {
		return Struct{}, false
	}
	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return Struct{}, false
	}
	named, _ := obj.Type().(*types.Named) // what every type declaration but an alias declares

	doc := ts.Doc
	if doc == nil && len(gd.Specs) == 1 {
		doc = gd.Doc
	}

	fields := make([]Field, st.NumFields())
	for i := range st.NumFields() {
		v := st.Field(i)
		fields[i] = Field{
			Name:     v.Name(),
			Type:     v.Type(),
			Tag:      reflect.StructTag(st.Tag(i)),
			Pos:      pkg.Fset.Position(v.Pos()),
			Exported: v.Exported(),
			Embedded: v.Embedded(),
		}
	}

	return Struct{
		PkgPath: pkg.PkgPath,
		PkgName: pkg.Name,
		Name:    obj.Name(),
		Named:   named,
		Pos:     pkg.Fset.Position(obj.Pos()),
		Doc:     doc,
		Fields:  fields,
	}, true
}
