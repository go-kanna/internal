package directive

import (
	"fmt"
	"slices"
	"strings"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/suggest"
)

// The arguments of a directive follow its key, separated by space. Each is a
// word, such as strict, skip-locked, or tenant_id,email, or key=value, whose
// value is all that follows the first =, such as model-package=../model.
// Every generator reads them so, through Parse, which says what is wrong with
// them in the same words for each.

// Arg is one argument of a directive.
type Arg struct {
	// Name is the word, or the key of key=value.
	Name string

	// Value is what follows the =, for key=value.
	Value    string
	HasValue bool
}

// Spec is what a directive takes after its key.
type Spec struct {
	// Words are the words it takes alone, and Keys the keys it takes a value
	// for.
	Words, Keys []string

	// Any, when set, names any other argument it takes, such as <field>, as a
	// word when AnyWord is set, as key=value when AnyKey is, or as either.
	Any             string
	AnyWord, AnyKey bool
}

// Parse splits the arguments of d into what spec takes, in the order they are
// written, and reports at d each argument spec does not take, takes in the
// other form, or takes once and finds again. What it reports is left out of
// what it returns.
func (d Directive) Parse(spec Spec) ([]Arg, []diag.Diag) {
	var (
		args  []Arg
		diags []diag.Diag
		seen  = make(map[string]bool)
	)
	for item := range strings.FieldsSeq(d.Args) {
		name, value, hasValue := strings.Cut(item, "=")
		a := Arg{Name: name, Value: value, HasValue: hasValue}
		if bad, ok := d.check(a, item, spec); ok {
			diags = append(diags, bad)
			continue
		}
		if seen[name] {
			diags = append(diags, diag.Errorf(d.Pos, "%s takes %s once", spelled(d.Key, ""), name))
			continue
		}
		seen[name] = true
		args = append(args, a)
	}
	return args, diags
}

// check reports what is wrong with a, written as item, under spec, if anything.
func (d Directive) check(a Arg, item string, spec Spec) (diag.Diag, bool) {
	word, key := slices.Contains(spec.Words, a.Name), slices.Contains(spec.Keys, a.Name)
	switch {
	case !a.HasValue && (word || spec.AnyWord && !key):
		return diag.Diag{}, false
	case a.HasValue && a.Value != "" && (key || spec.AnyKey && !word):
		return diag.Diag{}, false
	case word:
		return diag.Errorf(d.Pos, "%s takes %s alone, not %q", spelled(d.Key, ""), a.Name, item), true
	case key || a.HasValue && spec.AnyKey:
		return diag.Errorf(d.Pos, "%s takes a value for %s, as in %s=…", spelled(d.Key, ""), a.Name, a.Name), true
	}

	bad := diag.Errorf(d.Pos, "%s takes %s, not %q", spelled(d.Key, ""), spec.describe(), item)
	if s, ok := suggest.Closest(a.Name, append(slices.Clip(spec.Words), spec.Keys...)); ok {
		bad = bad.WithHints(fmt.Sprintf("did you mean %q?", s))
	}
	return bad, true
}

// describe spells out what spec takes, such as "must, name=…, or returns=…".
func (spec Spec) describe() string {
	var all []string
	all = append(all, spec.Words...)
	for _, k := range spec.Keys {
		all = append(all, k+"=…")
	}
	if spec.Any != "" && spec.AnyWord {
		all = append(all, spec.Any)
	}
	if spec.Any != "" && spec.AnyKey {
		all = append(all, spec.Any+"=…")
	}
	switch len(all) {
	case 0:
		return "no argument"
	case 1:
		return all[0]
	case 2:
		return all[0] + " or " + all[1]
	}
	return strings.Join(all[:len(all)-1], ", ") + ", or " + all[len(all)-1]
}
