package directive_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/directive"
)

// doc parses lines as the comment on a type declaration in p.go, where the
// first of them is line 3.
func doc(t *testing.T, lines []string) (*token.FileSet, *ast.CommentGroup) {
	t.Helper()

	src := "package p\n\n" + strings.Join(lines, "\n") + "\ntype T struct{}\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	gd, ok := f.Decls[0].(*ast.GenDecl)
	if !ok {
		t.Fatalf("first declaration is %T, want *ast.GenDecl", f.Decls[0])
	}
	return fset, gd.Doc
}

func strs(ds []diag.Diag) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.String())
	}
	return out
}

func TestFind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lines    []string
		key      string
		wantArgs string
		wantLine int // 0 when nothing is found
		want     []string
	}{
		{name: "no comment", key: "ignore"},
		{name: "unrelated comment", lines: []string{"// T stands in for a real record."}, key: "ignore"},
		{name: "marker only", lines: []string{"//kanna:ignore"}, key: "ignore", wantLine: 3},
		{
			name:     "arguments",
			lines:    []string{"//kanna:container name=NewFoo must"},
			key:      "container",
			wantArgs: "name=NewFoo must",
			wantLine: 3,
		},
		{name: "trailing space", lines: []string{"//kanna:ignore   "}, key: "ignore", wantLine: 3},
		{
			name:     "tab before the arguments",
			lines:    []string{"//kanna:container\tname=NewFoo"},
			key:      "container",
			wantArgs: "name=NewFoo",
			wantLine: 3,
		},
		{
			name:     "among other comment lines",
			lines:    []string{"// T is a person.", "//kanna:ignore", "// Not generated."},
			key:      "ignore",
			wantLine: 4,
		},
		{name: "another generator's key", lines: []string{"//kanna:container"}, key: "ignore"},
		{name: "longer key sharing the prefix", lines: []string{"//kanna:ignoreXYZ"}, key: "ignore"},
		{name: "block comment", lines: []string{"/* kanna:ignore */"}, key: "ignore"},
		{name: "prose mentioning the directive", lines: []string{"// Mark it kanna:ignore to skip it."}, key: "ignore"},
		{
			name:  "space after the comment marker",
			lines: []string{"// kanna:ignore"},
			key:   "ignore",
			want: []string{
				`p.go:3:1: warning: "// kanna:ignore" is not recognized as a directive; ` +
					`write it as "//kanna:ignore" with no space`,
			},
		},
		{
			// Following the suggestion must not cost the author the arguments.
			name:  "several spaces and arguments",
			lines: []string{"//   kanna:container name=A"},
			key:   "container",
			want: []string{
				`p.go:3:1: warning: "//   kanna:container name=A" is not recognized as a directive; ` +
					`write it as "//kanna:container name=A" with no space`,
			},
		},
		{
			name:     "spaced line before the real one",
			lines:    []string{"// kanna:ignore", "//kanna:ignore"},
			key:      "ignore",
			wantLine: 4,
			want: []string{
				`p.go:3:1: warning: "// kanna:ignore" is not recognized as a directive; ` +
					`write it as "//kanna:ignore" with no space`,
			},
		},
		{
			name:     "duplicate keeps the first",
			lines:    []string{"//kanna:container name=A", "//kanna:container name=B"},
			key:      "container",
			wantArgs: "name=A",
			wantLine: 3,
			want:     []string{"p.go:4:1: error: duplicate //kanna:container directive\n  the first is on line 3"},
		},
		{
			name:  "typo in the key",
			lines: []string{"//kanna:validte"},
			key:   "validate",
			want: []string{
				`p.go:3:1: warning: "//kanna:validte" is not recognized as a directive` + "\n" +
					`  did you mean "//kanna:validate"?`,
			},
		},
		{
			name:  "swapped letters and arguments",
			lines: []string{"//kanna:contianer name=A"},
			key:   "container",
			want: []string{
				`p.go:3:1: warning: "//kanna:contianer name=A" is not recognized as a directive` + "\n" +
					`  did you mean "//kanna:container name=A"?`,
			},
		},
		{
			// The suggestion fixes the spacing as well.
			name:  "typo after a space",
			lines: []string{"// kanna:validte"},
			key:   "validate",
			want: []string{
				`p.go:3:1: warning: "// kanna:validte" is not recognized as a directive` + "\n" +
					`  did you mean "//kanna:validate"?`,
			},
		},
		{name: "two typos away", lines: []string{"//kanna:vlidte"}, key: "validate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fset, cg := doc(t, tt.lines)
			got, ds := directive.Find(fset, cg, tt.key)

			if got.Found != (tt.wantLine > 0) || got.Args != tt.wantArgs || got.Pos.Line != tt.wantLine {
				t.Errorf("Find() = %+v, want args %q on line %d", got, tt.wantArgs, tt.wantLine)
			}
			if got := strs(ds); !slices.Equal(got, tt.want) {
				t.Errorf("diagnostics:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestFind_NilDoc(t *testing.T) {
	t.Parallel()

	got, ds := directive.Find(token.NewFileSet(), nil, "ignore")
	if got.Found || len(ds) > 0 {
		t.Errorf("Find(nil) = %+v, %v, want nothing", got, ds)
	}
}
