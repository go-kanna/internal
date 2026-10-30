package valuetype_test

import (
	"go/types"
	"testing"

	"github.com/go-kanna/internal/pkgtest"
	"github.com/go-kanna/internal/valuetype"
)

const src = `package test

import "time"

// CompletedAt is a value type as kanna-value generates one.
type CompletedAt struct{ t time.Time }

func (c CompletedAt) Time() time.Time { return c.t }

func NewCompletedAt(t time.Time) (CompletedAt, error) { return CompletedAt{t: t}, nil }

type Email struct{ s string }

func (e Email) String() string { return e.s }

func NewEmail(s string) (Email, error) { return Email{s: s}, nil }

type cents int64

type ünits int64

type price struct{ c cents }

func (p price) Cents() cents { return p.c }

func newPrice(c cents) (price, error) { return price{c: c}, nil }

// Price is price as another name, which is the same type.
type Price = price

// Bare has the shape but no constructor, as a value type of a package loaded
// only in part seems.
type Bare struct{ s string }

func (b Bare) String() string { return b.s }

// Infallible's constructor cannot fail, Mismatched's takes another type, and
// Twice's is variadic: none is kanna-value's.
type Infallible struct{ s string }

func (i Infallible) String() string { return i.s }

func NewInfallible(s string) Infallible { return Infallible{s: s} }

type Mismatched struct{ s string }

func (m Mismatched) String() string { return m.s }

func NewMismatched(n int) (Mismatched, error) { return Mismatched{}, nil }

type Twice struct{ s string }

func (t Twice) String() string { return t.s }

func NewTwice(s ...string) (Twice, error) { return Twice{}, nil }

// The rest are no value types at all.

type Pair struct{ a, b string }

type Exported struct{ S string }

func (e Exported) String() string { return e.S }

type Embedded struct{ time.Time }

type Unnamed struct{ s []string }

type NoAccessor struct{ s string }

type WrongAccessor struct{ s string }

func (w WrongAccessor) String() int { return 0 }

type Box[T any] struct{ v T }

type ID string

func (id ID) String() string { return string(id) }
`

func TestOf(t *testing.T) {
	t.Parallel()

	scope := pkgtest.Load(t, map[string]string{"test.go": src}).Types.Scope()

	tests := []struct {
		name     string
		accessor string // "" for no value type
		ctor     string // "" for none found
	}{
		{name: "CompletedAt", accessor: "Time", ctor: "NewCompletedAt"},
		{name: "Email", accessor: "String", ctor: "NewEmail"},
		{name: "price", accessor: "Cents", ctor: "newPrice"},
		{name: "Price", accessor: "Cents", ctor: "newPrice"},
		{name: "Bare", accessor: "String"},
		{name: "Infallible", accessor: "String"},
		{name: "Mismatched", accessor: "String"},
		{name: "Twice", accessor: "String"},
		{name: "Pair"},
		{name: "Exported"},
		{name: "Embedded"},
		{name: "Unnamed"},
		{name: "NoAccessor"},
		{name: "WrongAccessor"},
		{name: "Box"},
		{name: "ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v, ok := valuetype.Of(scope.Lookup(tt.name).Type())
			if ok != (tt.accessor != "") {
				t.Fatalf("Of(%s) reports %t", tt.name, ok)
			}
			if !ok {
				return
			}
			if v.Accessor != tt.accessor {
				t.Errorf("Accessor = %q, want %q", v.Accessor, tt.accessor)
			}
			if got := ctorName(v.Constructor); got != tt.ctor {
				t.Errorf("Constructor = %q, want %q", got, tt.ctor)
			}
		})
	}
}

func ctorName(fn *types.Func) string {
	if fn == nil {
		return ""
	}
	return fn.Name()
}

func TestAccessor(t *testing.T) {
	t.Parallel()

	scope := pkgtest.Load(t, map[string]string{"test.go": src}).Types.Scope()

	tests := []struct {
		held types.Type
		want string // "" for none
	}{
		{held: types.Typ[types.String], want: "String"},
		{held: types.Typ[types.Int64], want: "Int64"},
		{held: types.Universe.Lookup("byte").Type(), want: "Byte"},
		{held: scope.Lookup("CompletedAt").Type(), want: "CompletedAt"},
		{held: scope.Lookup("cents").Type(), want: "Cents"},
		{held: scope.Lookup("ünits").Type(), want: "Ünits"},
		{held: types.NewSlice(types.Typ[types.String])},
		{held: types.NewPointer(types.Typ[types.Int])},
	}

	for _, tt := range tests {
		t.Run(tt.held.String(), func(t *testing.T) {
			t.Parallel()

			got, ok := valuetype.Accessor(tt.held)
			if ok != (tt.want != "") || got != tt.want {
				t.Errorf("Accessor(%s) = %q, %t; want %q", tt.held, got, ok, tt.want)
			}
		})
	}
}

func TestConstructors(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, newFunc, mustFunc string }{
		{name: "Email", newFunc: "NewEmail", mustFunc: "MustEmail"},
		{name: "price", newFunc: "newPrice", mustFunc: "mustPrice"},
		// A letter of more than one byte is capitalized whole.
		{name: "éclair", newFunc: "newÉclair", mustFunc: "mustÉclair"},
		{name: "Ωmega", newFunc: "NewΩmega", mustFunc: "MustΩmega"},
		// A letter without case stays as it is, and so unexported.
		{name: "価格", newFunc: "new価格", mustFunc: "must価格"}, //nolint:gosmopolitan // a script without case is the case tested
	} {
		if newFunc, mustFunc := valuetype.Constructors(tt.name); newFunc != tt.newFunc || mustFunc != tt.mustFunc {
			t.Errorf("Constructors(%q) = %q, %q; want %q, %q", tt.name, newFunc, mustFunc, tt.newFunc, tt.mustFunc)
		}
	}
}
