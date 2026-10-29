package packages

import (
	"go/token"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/go-kanna/internal/diag"
)

// Split tells the errors in s.Diags that generating cannot mend from those it
// may. Code calling what a run is about to generate does not compile before
// the run, nor does code calling it as the run is about to change it, so a type
// error of a root package, one of s.Packages, is deferred to Recheck, which
// tells once the outputs are in place whether it remains. Every other error
// blocks: one of a package outside the main module, any error but a type error,
// and every error of a package that has one of those too.
//
// A type error of a package of the main module that is not a root is deferred
// too, though nothing checks it again: under go generate ./..., such a package
// is generated on a run of its own, which mends or reports it, and a run that
// imports it should not stop on it before, as when its code calls what that run
// is about to write.
//
// A generator that stops on blocking reports SetAsideWarnings along with it.
// One that goes on reads packages that may not type-check, so it passes over
// what has an Invalid type: the type error behind it is reported anyway, by
// Recheck, or by Stopped when the generator stops on errors of its own.
func (s Set) Split() (blocking, deferred []diag.Diag) {
	roots := make(map[*Package]bool, len(s.Packages))
	for _, p := range s.Packages {
		roots[p] = true
	}
	mendable := make(map[string]bool)
	packages.Visit(s.Packages, nil, func(p *Package) {
		if !roots[p] && (p.Module == nil || !p.Module.Main) ||
			slices.ContainsFunc(p.Errors, func(e packages.Error) bool { return !isTypeError(e) }) {
			return
		}
		for _, f := range p.CompiledGoFiles {
			mendable[f] = true
		}
	})

	for _, d := range s.Diags {
		if mendable[d.Pos.Filename] {
			deferred = append(deferred, d)
		} else {
			blocking = append(blocking, d)
		}
	}
	return blocking, deferred
}

// isTypeError reports whether e is a type error, which the compiler's account
// of the type errors, a list error diagnose passes over, counts as too.
func isTypeError(e packages.Error) bool {
	return e.Kind == packages.TypeError || e.Kind == packages.ListError && strings.HasPrefix(e.Msg, "# ")
}

// SetAsideWarnings warns, of each file in s.SetAside, that it is regenerated
// once the errors a run stops on are fixed.
func (s Set) SetAsideWarnings() []diag.Diag {
	ds := make([]diag.Diag, 0, len(s.SetAside))
	for _, path := range s.SetAside {
		ds = append(ds, diag.Warningf(token.Position{Filename: path},
			"set aside because it no longer compiles; it is regenerated once the errors above are fixed"))
	}
	return ds
}
