package validation

import (
	"go/token"
	"slices"
	"strings"

	"github.com/go-kanna/internal/deps"
	"github.com/go-kanna/internal/qualifier"
	"github.com/go-kanna/internal/scan"
)

// A qualified rule=, such as rule=model.CompletedAt, names its package as Go
// reads model.CompletedAt in code: by the name the struct's file imports it
// under, or by the package's own name where the file imports it as _, which a
// package that only a tag names has to be. See package qualifier.

// splitRef splits ref, a type as rule= names it, into the qualifier naming its
// package, "" for the struct's own, and the type's name. The name follows the
// last dot, since a path written instead of a name, which the caller reports,
// may hold dots of its own. It reports false for what cannot name a type.
func splitRef(ref string) (qual, name string, ok bool) {
	i := strings.LastIndex(ref, ".")
	if i < 0 {
		return "", ref, token.IsIdentifier(ref)
	}
	qual, name = ref[:i], ref[i+1:]
	if slices.Contains(strings.Split(qual, "/"), "") {
		return "", "", false
	}
	return qual, name, token.IsIdentifier(name)
}

// RuleRefs returns the packages the qualified rule= tags of s name, which a
// run that plans s reads for no import of its own saying so. Each names the
// package as the struct's file imports it.
func RuleRefs(s scan.Struct) []deps.Ref {
	imports := qualifier.Of(s.File, s.Named.Obj().Pkg())
	var refs []deps.Ref
	for _, ref := range ruleRefs(s) {
		if qual, _, ok := splitRef(ref); ok && qual != "" {
			if pkg, err := imports.Lookup(qual); err == nil {
				refs = append(refs, deps.Ref{Package: pkg.Path(), Via: "rule=" + ref})
			}
		}
	}
	return refs
}

// ruleRefs returns what the rule= tags of s name, as written.
func ruleRefs(s scan.Struct) []string {
	var refs []string
	for _, f := range s.Fields {
		levels, _ := parseTag(f.Tag)
		for _, l := range levels {
			for _, r := range l.rules {
				if r.name == "rule" {
					refs = append(refs, r.param)
				}
			}
		}
	}
	return refs
}
