package directive_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/go-kanna/internal/directive"
)

const shapeSrc = `package p

//kanna:config
type Struct struct {
	//kanna:config
	Field string
}

//kanna:config
type Defined Struct

//kanna:config
type Iface interface {
	//kanna:config
	Method()
	//kanna:config
	error
}

//kanna:config
type Level string

//kanna:config
type Alias = string

//kanna:config
type _ interface{}

//kanna:config
func F() {
	//kanna:config
	type local struct{}
}

//kanna:config
func (Struct) M() {}

//kanna:config
var V = func() {
	//kanna:config
	var x int
	_ = x
}

//kanna:config
const C = 1

type (
	//kanna:config
	InGroup struct{}

	Other struct{}
)

//kanna:config
type (
	A struct{}
	B struct{}
)

//kanna:config
type (
	One struct{}
)

//kanna:config

type Loose struct{}
`

func TestLine_Shape(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", shapeSrc, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("p", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lines := directive.Lines(fset, []*ast.File{f})

	type shape struct {
		row   int
		shape directive.Shape
	}
	var got []shape
	for _, l := range lines {
		got = append(got, shape{l.Pos.Line, l.Shape(pkg)})
	}
	want := []shape{
		{3, directive.StructType}, {5, directive.Field}, {9, directive.StructType}, {12, directive.InterfaceType},
		{14, directive.InterfaceMethod}, {16, directive.InterfaceEmbed}, {20, directive.OtherType},
		{23, directive.AliasType}, {26, directive.InterfaceType}, {29, directive.Func}, {31, directive.Local},
		{35, directive.Method}, {38, directive.Var}, {40, directive.Local}, {45, directive.Const},
		{49, directive.StructType}, {55, directive.Group}, {61, directive.StructType}, {66, 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Shape() =\n%v\nwant\n%v", got, want)
	}

	// Without the package, a type declaration is decided by its syntax.
	if got := lines[2].Shape(nil); got != directive.OtherType {
		t.Errorf("Shape(nil) of type Defined Struct = %v, want OtherType", got)
	}
}

func TestMisplaced(t *testing.T) {
	t.Parallel()

	const src = `package p

//kanna:container
type Server struct{}

//kanna:container
type Port int

//kanna:container
func NewThing() int { return 1 }

//kanna:validate
func f() {}

//kanna:container

func g() {}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("p", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lines := directive.Lines(fset, []*ast.File{f})

	if got, want := strs(directive.Misplaced(lines, pkg, directive.StructType, "container")), []string{
		"p.go:6:1: error: //kanna:container marks a struct type, not a type other than a struct or an interface, " +
			"so nothing reads it\n  write it right above a struct type",
		"p.go:9:1: error: //kanna:container marks a struct type, not a function, so nothing reads it\n" +
			"  write it right above a struct type",
	}; !slices.Equal(got, want) {
		t.Errorf("Misplaced() =\n%q\nwant\n%q", got, want)
	}

	shapes := directive.StructType | directive.InterfaceType
	if got, want := strs(directive.Misplaced(lines, pkg, shapes, "validate")), []string{
		"p.go:12:1: error: //kanna:validate marks a struct type or an interface type, not a function, " +
			"so nothing reads it\n  write it right above a struct type or an interface type",
	}; !slices.Equal(got, want) {
		t.Errorf("Misplaced() =\n%q\nwant\n%q", got, want)
	}
}
