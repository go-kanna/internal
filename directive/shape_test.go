package directive_test

import (
	"go/ast"
	"go/importer"
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
		"p.go:6:1: error: //kanna:container marks a struct type, not an int type, so nothing reads it\n" +
			"  write it right above a struct type",
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

func TestMisplaced_OtherType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		decl     string
		want     string
		withoutT string
	}{
		{"type T string", "a string type", "a type other than a struct or an interface"},
		{"type T uint8", "a uint8 type", "a type other than a struct or an interface"},
		{"type T Base", "an int64 type", "a type other than a struct or an interface"},
		{"type T unsafe.Pointer", "an unsafe.Pointer type", "a type other than a struct or an interface"},
		{"type T Undeclared", "a type other than a struct or an interface", "a type other than a struct or an interface"},
		{"type T []int", "a slice type", "a slice type"},
		{"type T [2]int", "an array type", "an array type"},
		{"type T map[string]int", "a map type", "a map type"},
		{"type T *int", "a pointer type", "a pointer type"},
		{"type T (func())", "a function type", "a function type"},
		{"type T chan int", "a channel type", "a channel type"},
	}
	for _, tt := range tests {
		t.Run(tt.decl, func(t *testing.T) {
			t.Parallel()

			src := "package p\n\nimport \"unsafe\"\n\nvar _ unsafe.Pointer\n\ntype Base int64\n\n//kanna:container\n" +
				tt.decl + "\n"
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			// Undeclared fails to check, and the package keeps what it knows.
			conf := types.Config{Importer: importer.Default(), Error: func(error) {}}
			pkg, _ := conf.Check("p", fset, []*ast.File{f}, nil)
			lines := directive.Lines(fset, []*ast.File{f})

			for _, c := range []struct {
				pkg  *types.Package
				want string
			}{{pkg, tt.want}, {nil, tt.withoutT}} {
				want := []string{"p.go:9:1: error: //kanna:container marks a struct type, not " + c.want +
					", so nothing reads it\n  write it right above a struct type"}
				if got := strs(directive.Misplaced(lines, c.pkg, directive.StructType, "container")); !slices.Equal(got, want) {
					t.Errorf("Misplaced(pkg=%v) =\n%q\nwant\n%q", c.pkg != nil, got, want)
				}
			}
		})
	}
}
