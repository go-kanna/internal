package packages

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/go-kanna/internal/diag"
)

// notTypeChecked is the message of NotTypeChecked.
const notTypeChecked = "does not type-check"

// Invalid reports whether t does not type-check, as a type a package with type
// errors declares or uses may not: an undefined Missing makes *Missing invalid
// too. A generator going on past Split passes over such a type, saying
// NotTypeChecked of what it would have generated from it.
func Invalid(t types.Type) bool {
	switch t := t.(type) {
	case *types.Basic:
		return t.Kind() == types.Invalid
	case *types.Pointer:
		return Invalid(t.Elem())
	case *types.Slice:
		return Invalid(t.Elem())
	case *types.Array:
		return Invalid(t.Elem())
	case *types.Map:
		return Invalid(t.Key()) || Invalid(t.Elem())
	case *types.Chan:
		return Invalid(t.Elem())
	}
	return false
}

// NotTypeChecked is the error a generator reports at pos of what it cannot
// generate as what it reads there does not type-check. It stops the run, but
// Stopped leaves it out, as the type error behind it says more.
func NotTypeChecked(pos token.Position) diag.Diag {
	return diag.Errorf(pos, "%s", notTypeChecked)
}

// Stopped returns what a run reports when it stops on ds, errors of its own,
// before Recheck can tell: ds but what NotTypeChecked says, and the type errors
// Split deferred, as one may be the cause, but for those pending tells, such as
// those Set.Pending does. Should that leave no error, it returns them all, so
// that a run never stops without saying why.
func Stopped(deferred, ds []diag.Diag, pending func(diag.Diag) bool) []diag.Diag {
	reported := slices.Concat(
		slices.DeleteFunc(slices.Clone(deferred), pending),
		slices.DeleteFunc(slices.Clone(ds), func(d diag.Diag) bool { return d.Message == notTypeChecked }),
	)
	if !diag.HasErrors(reported) {
		return slices.Concat(deferred, ds)
	}
	return reported
}
