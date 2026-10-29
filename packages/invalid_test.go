package packages_test

import (
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/packages"
)

func TestInvalid(t *testing.T) {
	t.Parallel()

	invalid := types.Typ[types.Invalid]
	named := types.NewNamed(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewStruct(nil, nil), nil)

	tests := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{"invalid", invalid, true},
		{"a pointer", types.NewPointer(invalid), true},
		{"a slice", types.NewSlice(invalid), true},
		{"an array", types.NewArray(invalid, 2), true},
		{"a map's key", types.NewMap(invalid, types.Typ[types.Int]), true},
		{"a map's element", types.NewMap(types.Typ[types.String], invalid), true},
		{"a channel", types.NewChan(types.SendRecv, invalid), true},
		{"a basic type", types.Typ[types.Int], false},
		{"a pointer to one", types.NewPointer(types.Typ[types.Int]), false},
		{"a named type", named, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := packages.Invalid(tt.typ); got != tt.want {
				t.Errorf("Invalid(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestStopped(t *testing.T) {
	t.Parallel()

	at := func(line int) token.Position { return token.Position{Filename: "a.go", Line: line} }
	var (
		pending  = diag.Errorf(at(1), "undefined: F")
		typeErr  = diag.Errorf(at(2), "undefined: Missing")
		notTC    = packages.NotTypeChecked(at(3))
		own      = diag.Errorf(at(4), "unknown rule")
		warning  = diag.Warningf(at(5), "never checked")
		isF      = func(d diag.Diag) bool { return d.Message == pending.Message }
		messages = func(ds []diag.Diag) []string {
			out := make([]string, 0, len(ds))
			for _, d := range ds {
				out = append(out, d.String())
			}
			return out
		}
	)

	tests := []struct {
		name         string
		deferred, ds []diag.Diag
		want         []diag.Diag
	}{
		{
			name:     "the type errors that say more, and the run's own",
			deferred: []diag.Diag{pending, typeErr},
			ds:       []diag.Diag{notTC, own, warning},
			want:     []diag.Diag{typeErr, own, warning},
		},
		{
			// A run never stops without an error to show.
			name:     "nothing left to say",
			deferred: []diag.Diag{pending},
			ds:       []diag.Diag{notTC, warning},
			want:     []diag.Diag{pending, notTC, warning},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := packages.Stopped(tt.deferred, tt.ds, isF)
			if !slices.Equal(messages(got), messages(tt.want)) {
				t.Errorf("Stopped() = %q, want %q", messages(got), messages(tt.want))
			}
		})
	}
}
