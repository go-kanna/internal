package enumtype_test

import (
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

	scope := pkgtest.Load(t, map[string]string{"test.go": src}).Types.Scope()

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
			for _, c := range e.Constants {
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
