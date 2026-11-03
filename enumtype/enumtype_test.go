package enumtype_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/go-kanna/internal/enumtype"
	"github.com/go-kanna/internal/pkgtest"
)

const src = `package test

// Status is an enum as kanna-value generates one, its constants declared out
// of alphabetical order.
type Status string

const (
	StatusPlaced  Status = "placed"
	StatusPaid    Status = "paid"
	statusHidden  Status = "hidden"
	StatusShipped Status = "shipped"
)

func NewStatus(v string) (Status, error) { return Status(v), nil }

type kind string

const kindA kind = "a"

func newKind(v string) (kind, error) { return kind(v), nil }

// Kind is kind as another name, which is the same type.
type Kind = kind

// Bare has constants but no constructor, as an enum of a package loaded only
// in part seems.
type Bare string

const BareA Bare = "a"

// Infallible's constructor cannot fail, and Mismatched's takes another type:
// neither is kanna-value's.
type Infallible string

const InfallibleA Infallible = "a"

func NewInfallible(v string) Infallible { return Infallible(v) }

type Mismatched string

const MismatchedA Mismatched = "a"

func NewMismatched(v []byte) (Mismatched, error) { return Mismatched(v), nil }

// The rest are no enums at all.

type NoConstants string

func NewNoConstants(v string) (NoConstants, error) { return NoConstants(v), nil }

// Untyped's constant is untyped, and Derived's constant is of the type it
// derives from.
type Untyped string

const UntypedA = "a"

type Derived Status

type Number int

const NumberA Number = 1

type Gen[T any] string
`

func TestOf(t *testing.T) {
	t.Parallel()

	pkg := pkgtest.Load(t, map[string]string{"test.go": src})
	scope := pkg.Types.Scope()

	tests := []struct {
		name   string
		consts []string // nil for no enum
		ctor   string   // "" for none found
	}{
		{name: "Status", consts: []string{"StatusPlaced", "StatusPaid", "statusHidden", "StatusShipped"}, ctor: "NewStatus"},
		{name: "kind", consts: []string{"kindA"}, ctor: "newKind"},
		{name: "Kind", consts: []string{"kindA"}, ctor: "newKind"},
		{name: "Bare", consts: []string{"BareA"}},
		{name: "Infallible", consts: []string{"InfallibleA"}},
		{name: "Mismatched", consts: []string{"MismatchedA"}},
		{name: "NoConstants"},
		{name: "Untyped"},
		{name: "Derived"},
		{name: "Number"},
		{name: "Gen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e, ok := enumtype.Of(scope.Lookup(tt.name).Type())
			if ok != (tt.consts != nil) {
				t.Fatalf("Of(%s) reports %t", tt.name, ok)
			}
			if !ok {
				return
			}
			var consts []string
			for _, c := range enumtype.Constants(e.Named, pkg.Fset) {
				consts = append(consts, c.Name())
			}
			if !slices.Equal(consts, tt.consts) {
				t.Errorf("Constants = %q, want %q", consts, tt.consts)
			}
			if got := ctorName(e.Constructor); got != tt.ctor {
				t.Errorf("Constructor = %q, want %q", got, tt.ctor)
			}
		})
	}
}

// TestConstants_FileByFile parses the files out of name order, as go/packages,
// which parses them concurrently, may.
func TestConstants_FileByFile(t *testing.T) {
	t.Parallel()

	files := []struct{ name, src string }{
		{"b.go", "package test\n\nconst (\n\tLevelLow  Level = \"low\"\n\tLevelHigh Level = \"high\"\n)\n"},
		{"a.go", "package test\n\ntype Level string\n\nconst LevelMid Level = \"mid\"\n"},
	}
	fset := token.NewFileSet()
	syntax := make([]*ast.File, 0, len(files))
	for _, f := range files {
		file, err := parser.ParseFile(fset, f.name, f.src, 0)
		if err != nil {
			t.Fatal(err)
		}
		syntax = append(syntax, file)
	}
	pkg, err := (&types.Config{}).Check("test", fset, syntax, nil)
	if err != nil {
		t.Fatal(err)
	}

	e, ok := enumtype.Of(pkg.Scope().Lookup("Level").Type())
	if !ok {
		t.Fatal("Of(Level) reports false")
	}
	consts := enumtype.Constants(e.Named, fset)
	names := make([]string, len(consts))
	for i, c := range consts {
		names[i] = c.Name()
	}
	if want := []string{"LevelMid", "LevelLow", "LevelHigh"}; !slices.Equal(names, want) {
		t.Errorf("Constants = %q, want %q", names, want)
	}
}

func ctorName(fn *types.Func) string {
	if fn == nil {
		return ""
	}
	return fn.Name()
}

func TestFuncs(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, newFunc, valuesFunc string }{
		{name: "OrderStatus", newFunc: "NewOrderStatus", valuesFunc: "OrderStatusValues"},
		{name: "orderStatus", newFunc: "newOrderStatus", valuesFunc: "orderStatusValues"},
		{name: "état", newFunc: "newÉtat", valuesFunc: "étatValues"},
	} {
		if newFunc, valuesFunc := enumtype.Funcs(tt.name); newFunc != tt.newFunc || valuesFunc != tt.valuesFunc {
			t.Errorf("Funcs(%q) = %q, %q; want %q, %q", tt.name, newFunc, valuesFunc, tt.newFunc, tt.valuesFunc)
		}
	}
}
