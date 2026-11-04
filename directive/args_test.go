package directive_test

import (
	"slices"
	"testing"

	"github.com/go-kanna/internal/directive"
)

func TestDirective_Parse(t *testing.T) {
	t.Parallel()

	container := directive.Spec{Words: []string{"must"}, Keys: []string{"name", "returns"}}
	mapping := directive.Spec{Any: "<field>", AnyWord: true, AnyKey: true}
	shares := directive.Spec{Any: "<field>", AnyWord: true}

	tests := []struct {
		name string
		key  string
		args string
		spec directive.Spec
		want []directive.Arg
		errs []string
	}{
		{
			name: "words and values, in order",
			key:  "container", args: "name=NewFoo must", spec: container,
			want: []directive.Arg{{Name: "name", Value: "NewFoo", HasValue: true}, {Name: "must"}},
		},
		{
			// A value is all that follows the first =, and a word may hold
			// what a name does not.
			name: "what an argument holds",
			key:  "scopes", args: "model-package=../model=x tenant_id,email",
			spec: directive.Spec{Words: []string{"tenant_id,email"}, Keys: []string{"model-package"}},
			want: []directive.Arg{
				{Name: "model-package", Value: "../model=x", HasValue: true}, {Name: "tenant_id,email"},
			},
		},
		{
			name: "any field, either way",
			key:  "map", args: "ID Email=ContactEmail Internal=-", spec: mapping,
			want: []directive.Arg{
				{Name: "ID"}, {Name: "Email", Value: "ContactEmail", HasValue: true},
				{Name: "Internal", Value: "-", HasValue: true},
			},
		},
		{
			name: "a misspelled word",
			key:  "fixture", args: "skp", spec: directive.Spec{Words: []string{"skip"}},
			errs: []string{`p.go:3:1: error: //kanna:fixture takes skip, not "skp"` + "\n  did you mean \"skip\"?"},
		},
		{
			name: "a misspelled key",
			key:  "container", args: "nam=NewFoo", spec: container,
			errs: []string{`p.go:3:1: error: //kanna:container takes must, name=…, or returns=…, not "nam=NewFoo"` +
				"\n  did you mean \"name\"?"},
		},
		{
			name: "no argument taken",
			key:  "validate", args: "strict",
			errs: []string{`p.go:3:1: error: //kanna:validate takes no argument, not "strict"`},
		},
		{
			name: "a word given a value",
			key:  "container", args: "must=yes", spec: container,
			errs: []string{`p.go:3:1: error: //kanna:container takes must alone, not "must=yes"`},
		},
		{
			name: "a key without a value",
			key:  "container", args: "name name=", spec: container,
			errs: []string{
				"p.go:3:1: error: //kanna:container takes a value for name, as in name=…",
				"p.go:3:1: error: //kanna:container takes a value for name, as in name=…",
			},
		},
		{
			name: "any field as a word alone",
			key:  "share", args: "Tags Labels=x", spec: shares,
			want: []directive.Arg{{Name: "Tags"}},
			errs: []string{`p.go:3:1: error: //kanna:share takes <field>, not "Labels=x"`},
		},
		{
			name: "none where one is needed",
			key:  "fixture", spec: directive.Spec{Words: []string{"skip"}, Needed: true},
			errs: []string{"p.go:3:1: error: //kanna:fixture needs skip"},
		},
		{
			name: "twice",
			key:  "map", args: "Name=A Name=B", spec: mapping,
			want: []directive.Arg{{Name: "Name", Value: "A", HasValue: true}},
			errs: []string{"p.go:3:1: error: //kanna:map takes Name once"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fset, cg := doc(t, []string{"//kanna:" + tt.key + " " + tt.args})
			d, _ := directive.Find(fset, cg, tt.key)
			got, ds := d.Parse(tt.spec)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Parse() = %+v, want %+v", got, tt.want)
			}
			if got := strs(ds); !slices.Equal(got, tt.errs) {
				t.Errorf("diagnostics:\n%q\nwant:\n%q", got, tt.errs)
			}
		})
	}
}
