package directive_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"github.com/go-kanna/internal/directive"
)

const linesSrc = `//kanna:validate
package p

//kanna:validate

type Loose struct{}

//kanna:validate
type Marked struct {
	//kanna:map Title
	Name string
}

type Service interface {
	//kanna:map ID
	Get() string
}

//kanna:converter
var list = []any{}

//kanna:di
type Server struct{}

//kanna:validte
type Typo struct{}

//kanna:valdte

// kanna:validate stays prose.
func f() {
	//kanna:validate
}
`

func TestLines(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", linesSrc, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	lines := directive.Lines(fset, []*ast.File{f})

	type line struct {
		key      string
		row      int
		attached bool
	}
	got := make([]line, len(lines))
	for i, l := range lines {
		got[i] = line{key: l.Key, row: l.Pos.Line, attached: l.Attached}
	}
	want := []line{
		{"validate", 1, false}, {"validate", 4, false}, {"validate", 8, true}, {"map", 10, true},
		{"map", 15, true}, {"converter", 19, true}, {"di", 22, true}, {"validte", 25, true},
		{"valdte", 28, false}, {"validate", 32, false},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Lines() =\n%v\nwant\n%v", got, want)
	}

	if got, want := strs(directive.Loose(lines, "validate")), []string{
		"p.go:1:1: warning: //kanna:validate is not attached to a declaration, so nothing reads it\n" +
			"  write it right above what it marks, with no blank line between",
		"p.go:4:1: warning: //kanna:validate is not attached to a declaration, so nothing reads it\n" +
			"  write it right above what it marks, with no blank line between",
		"p.go:32:2: warning: //kanna:validate is not attached to a declaration, so nothing reads it\n" +
			"  write it right above what it marks, with no blank line between",
	}; !slices.Equal(got, want) {
		t.Errorf("Loose() =\n%q\nwant\n%q", got, want)
	}

	// //kanna:validte is on a declaration, where kanna-validator tells it.
	if got, want := strs(directive.Unknown(lines)), []string{
		"p.go:22:1: warning: //kanna:di is a directive of no kanna generator",
		"p.go:28:1: warning: //kanna:valdte is a directive of no kanna generator\n" +
			"  did you mean \"//kanna:validate\"?",
	}; !slices.Equal(got, want) {
		t.Errorf("Unknown() =\n%q\nwant\n%q", got, want)
	}
}
