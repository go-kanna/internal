package directive

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/suggest"
)

// Keys are the keys of the directives of every generator of the family, so that
// a line under none of them is told apart from one another generator reads.
// pluck is gone, but kanna-orm still reads it to say so.
var Keys = []string{
	"allow", "config", "container", "converter", "enum", "fixture", "map", "mapper", "share", "validate", "value",
	"conflict", "lock", "pluck", "repository", "scope", "scopes",
}

// Line is a //kanna:<key> line of a file.
type Line struct {
	Key string
	Pos token.Position

	// Attached reports whether the line is in the doc comment of a
	// declaration, a field, or a method, where a generator looks for it.
	Attached bool

	// shape is what the line is the doc of, and typ the type spec Shape
	// decides by its type, if any.
	shape Shape
	typ   *ast.TypeSpec
}

// Lines returns the //kanna: lines of files, in the order they are written. A
// line spaced as // kanna: is prose rather than a directive, and is left out.
func Lines(fset *token.FileSet, files []*ast.File) []Line {
	var lines []Line
	for _, f := range files {
		docs := docsOf(f)
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				body, ok := strings.CutPrefix(c.Text, "//")
				if !ok || !strings.HasPrefix(body, namespace) {
					continue
				}
				l := docs[cg]
				l.Key, _, _ = parse(body)
				l.Pos = fset.Position(c.Slash)
				lines = append(lines, l)
			}
		}
	}
	return lines
}

// Loose warns about each line of lines whose key is among keys, a generator's
// own, and that is in no doc comment, so that nothing reads it: most often a
// blank line between it and what it marks.
func Loose(lines []Line, keys ...string) []diag.Diag {
	var diags []diag.Diag
	for _, l := range lines {
		if !l.Attached && slices.Contains(keys, l.Key) {
			diags = append(diags, diag.Warningf(l.Pos, "%s is not attached to a declaration, so nothing reads it",
				spelled(l.Key, "")).WithHints("write it right above what it marks, with no blank line between"))
		}
	}
	return diags
}

// Unknown warns about each line of lines whose key no generator of the family
// has, which every one of them passes over. A line a typo away from a key,
// on a declaration, is left to the generator of that key, which Find has
// warn about it.
func Unknown(lines []Line) []diag.Diag {
	var diags []diag.Diag
	for _, l := range lines {
		near := slices.ContainsFunc(Keys, func(k string) bool { return suggest.Distance(l.Key, k) == 1 })
		if slices.Contains(Keys, l.Key) || l.Attached && near {
			continue
		}
		d := diag.Warningf(l.Pos, "%s is a directive of no kanna generator", spelled(l.Key, ""))
		if s, ok := suggest.Closest(l.Key, Keys); ok {
			d = d.WithHints(fmt.Sprintf("did you mean %q?", spelled(s, "")))
		}
		diags = append(diags, d)
	}
	return diags
}
