package directive

import (
	"go/ast"
	"go/token"
	"go/types"
	"math/bits"
	"slices"
	"strings"

	"github.com/go-kanna/internal/diag"
)

// Shape is the shape of the declaration a //kanna: line is the doc of. A
// generator names the shapes its key marks by joining them, as in
// StructType|InterfaceType.
type Shape uint

const (
	// StructType is a type declaration whose underlying type is a struct, as
	// type Order model.Order is.
	StructType Shape = 1 << iota
	// InterfaceType is a type declaration whose underlying type is an
	// interface.
	InterfaceType
	// OtherType is a type declaration of any other kind, as type Level string.
	OtherType
	// AliasType is a type alias, as type Level = string.
	AliasType
	Func
	Method
	Var
	Const
	Import
	// Field is a field of a struct, embedded ones included.
	Field
	// InterfaceMethod is a method of an interface.
	InterfaceMethod
	// InterfaceEmbed is a type embedded in an interface.
	InterfaceEmbed
	// Group is a parenthesized group of declarations other than one, whose
	// doc documents none of them.
	Group
	// Local is anything declared inside a function body.
	Local
)

var shapeNames = []string{
	"a struct type", "an interface type", "a type other than a struct or an interface", "a type alias",
	"a function", "a method", "a variable", "a constant", "an import", "a struct field", "a method of an interface",
	"a type embedded in an interface", "a group of declarations", "a declaration inside a function",
}

// String names the shapes of s, joined by "or".
func (s Shape) String() string {
	var names []string
	for s != 0 {
		i := bits.TrailingZeros(uint(s))
		if i < len(shapeNames) {
			names = append(names, shapeNames[i])
		}
		s &^= 1 << i
	}
	return strings.Join(names, " or ")
}

// Shape returns the shape of what l is the doc of, or 0 if l is not Attached.
// pkg is the package of the file of l, which decides a type declaration by the
// type it declares, so that type Order model.Order is a StructType. A type pkg
// does not know, such as the blank type _, is decided by its syntax.
func (l Line) Shape(pkg *types.Package) Shape {
	if l.typ == nil {
		return l.shape
	}
	if pkg != nil {
		if obj, ok := pkg.Scope().Lookup(l.typ.Name.Name).(*types.TypeName); ok && obj.Pos() == l.typ.Name.Pos() {
			if obj.IsAlias() {
				return AliasType
			}
			switch u := obj.Type().Underlying().(type) {
			case *types.Struct:
				return StructType
			case *types.Interface:
				return InterfaceType
			case *types.Basic:
				if u.Kind() == types.Invalid {
					return l.shape
				}
			}
			return OtherType
		}
	}
	return l.shape
}

// Misplaced tells each line of lines whose key is among keys, a generator's
// own, and that is the doc of a declaration of none of the shapes in accept,
// as //kanna:container on a function. The generator looks for the key on those
// shapes alone, so nothing reads it. pkg is the package of the files of lines,
// as for Line.Shape. A line in no doc comment is left to Loose.
//
// The line can do nothing however it is written, so it is an error. A
// generator reading //kanna:container on structs alone checks a package with
//
//	lines := directive.Lines(pkg.Fset, pkg.Syntax)
//	diags = append(diags, directive.Loose(lines, "container")...)
//	diags = append(diags, directive.Misplaced(lines, pkg.Types, directive.StructType, "container")...)
func Misplaced(lines []Line, pkg *types.Package, accept Shape, keys ...string) []diag.Diag {
	var diags []diag.Diag
	for _, l := range lines {
		if !l.Attached || !slices.Contains(keys, l.Key) {
			continue
		}
		if s := l.Shape(pkg); s&accept == 0 {
			diags = append(diags, diag.Errorf(l.Pos, "%s marks %s, not %s, so nothing reads it", spelled(l.Key, ""),
				accept, s).WithHints("write it right above "+accept.String()))
		}
	}
	return diags
}

// docsOf returns the doc comments of what f declares, down to the fields of
// its structs and the methods of its interfaces, each with the line its
// directives take: the shape it is the doc of, and the type spec Line.Shape
// decides by its type.
func docsOf(f *ast.File) map[*ast.CommentGroup]Line {
	docs := make(map[*ast.CommentGroup]Line)
	add := func(doc *ast.CommentGroup, l Line) {
		if doc != nil {
			l.Attached = true
			docs[doc] = l
		}
	}

	var walk func(n ast.Node, local bool)
	walk = func(n ast.Node, local bool) {
		ast.Inspect(n, func(n ast.Node) bool {
			if b, ok := n.(*ast.BlockStmt); ok && !local {
				walk(b, true)
				return false
			}
			at := func(s Shape) Line {
				if local {
					return Line{shape: Local}
				}
				return Line{shape: s}
			}

			switch n := n.(type) {
			case *ast.GenDecl:
				for _, spec := range n.Specs {
					l := specLine(n.Tok, spec)
					if local {
						l = Line{shape: Local}
					}
					add(specDoc(spec), l)
					if len(n.Specs) == 1 {
						add(n.Doc, l)
					}
				}
				if len(n.Specs) != 1 {
					add(n.Doc, at(Group))
				}
			case *ast.FuncDecl:
				if n.Recv != nil {
					add(n.Doc, at(Method))
				} else {
					add(n.Doc, at(Func))
				}
			case *ast.StructType:
				for _, f := range n.Fields.List {
					add(f.Doc, at(Field))
				}
			case *ast.InterfaceType:
				for _, f := range n.Methods.List {
					if len(f.Names) > 0 {
						add(f.Doc, at(InterfaceMethod))
					} else {
						add(f.Doc, at(InterfaceEmbed))
					}
				}
			}
			return true
		})
	}
	walk(f, false)
	return docs
}

// specLine is the line a directive in the doc of spec, a spec of a declaration
// of tok, takes.
func specLine(tok token.Token, spec ast.Spec) Line {
	switch spec := spec.(type) {
	case *ast.TypeSpec:
		switch {
		case spec.Assign.IsValid():
			return Line{shape: AliasType}
		case isType[*ast.StructType](spec.Type):
			return Line{shape: StructType, typ: spec}
		case isType[*ast.InterfaceType](spec.Type):
			return Line{shape: InterfaceType, typ: spec}
		}
		return Line{shape: OtherType, typ: spec}
	case *ast.ValueSpec:
		if tok == token.CONST {
			return Line{shape: Const}
		}
		return Line{shape: Var}
	}
	return Line{shape: Import}
}

func specDoc(spec ast.Spec) *ast.CommentGroup {
	switch spec := spec.(type) {
	case *ast.TypeSpec:
		return spec.Doc
	case *ast.ValueSpec:
		return spec.Doc
	case *ast.ImportSpec:
		return spec.Doc
	}
	return nil
}

func isType[T ast.Expr](e ast.Expr) bool {
	_, ok := e.(T)
	return ok
}
