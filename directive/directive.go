// Package directive finds the //kanna:<key> comments that mark a declaration
// for a generator.
//
// Every generator writes into one namespace and tells itself apart by the key
// that follows it: //kanna:validate, //kanna:container, //kanna:ignore. Reading
// them in one place keeps the syntax from drifting apart per generator.
package directive

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"unicode"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/suggest"
)

const namespace = "kanna:"

// Directive is the //kanna:<key> line Find found, if any.
type Directive struct {
	Found bool

	// Args is what followed the key on its line, with surrounding space
	// removed. It is empty for a bare marker.
	Args string

	// Pos is where the line starts, for reporting a problem with Args.
	Pos token.Position
}

// Find looks through doc for the //kanna:<key> line. doc may be nil.
//
// At most one such line is honored; a second is an error rather than a silent
// override. Two kinds of line are not honored but warned about, because the
// declaration would otherwise go unmarked without a word:
//
//   - one with space between the "//" and the tag. go/ast treats only the
//     unspaced form as a directive, so a spaced line stays in the doc comment
//     and shows in go doc and on pkg.go.dev, and keeping the namespace out of
//     documentation is the point of the unspaced form;
//   - one whose key is a single typo away from key.
func Find(fset *token.FileSet, doc *ast.CommentGroup, key string) (Directive, []diag.Diag) {
	if doc == nil {
		return Directive{}, nil
	}

	var (
		found Directive
		ds    []diag.Diag
	)
	for _, c := range doc.List {
		// A block comment is never a directive.
		body, ok := strings.CutPrefix(c.Text, "//")
		if !ok {
			continue
		}
		k, args, ok := parse(body)
		if !ok {
			continue
		}
		pos := fset.Position(c.Slash)
		written := strings.TrimSpace(c.Text)

		switch {
		case k != key:
			if suggest.Distance(k, key) == 1 {
				ds = append(ds, diag.Warningf(pos, "%q is not recognized as a directive", written).
					WithHints(fmt.Sprintf("did you mean %q?", spelled(key, args))))
			}
		case !strings.HasPrefix(body, namespace):
			ds = append(ds, diag.Warningf(pos, "%q is not recognized as a directive; write it as %q with no space",
				written, spelled(key, args)))
		case found.Found:
			ds = append(ds, diag.Errorf(pos, "duplicate %s directive", spelled(key, "")).
				WithHints(fmt.Sprintf("the first is on line %d", found.Pos.Line)))
		default:
			found = Directive{Found: true, Args: args, Pos: pos}
		}
	}
	return found, ds
}

// parse splits a comment body such as "kanna:container name=A" into its key and
// arguments, reporting whether the body is in the kanna namespace at all. The
// key runs up to the first space, so kanna:ignoreXYZ is a key of its own rather
// than ignore with something stuck to it.
func parse(body string) (key, args string, ok bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(body), namespace)
	if !ok {
		return "", "", false
	}
	if i := strings.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		return rest[:i], strings.TrimSpace(rest[i:]), true
	}
	return rest, "", true
}

// spelled renders the directive for key as it should be written.
func spelled(key, args string) string {
	if args == "" {
		return "//" + namespace + key
	}
	return "//" + namespace + key + " " + args
}
