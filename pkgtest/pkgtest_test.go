package pkgtest_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/pkgtest"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	pkg := pkgtest.Load(t, map[string]string{
		"b.go": "package model\n\nimport \"time\"\n\ntype B struct{ At time.Time }\n",
		"a.go": "package model\n\ntype A struct{ Next B }\n",
	})

	if pkg.Name != "model" || pkg.PkgPath != "model" {
		t.Errorf("Name, PkgPath = %q, %q, want model, model", pkg.Name, pkg.PkgPath)
	}
	if want := []string{"a.go", "b.go"}; !slices.Equal(pkg.GoFiles, want) {
		t.Errorf("GoFiles = %q, want %q", pkg.GoFiles, want)
	}
	// a.go is parsed first, so it holds the first positions.
	if got := pkg.Fset.Position(pkg.Syntax[0].Pos()).Filename; got != "a.go" {
		t.Errorf("first file = %s, want a.go", got)
	}
	if pkg.Types.Scope().Lookup("B") == nil {
		t.Error("B is not in the package scope")
	}
	if pkg.TypesInfo == nil {
		t.Fatal("TypesInfo = nil, want what the syntax refers to")
	}
	var fromTime []string
	for id, obj := range pkg.TypesInfo.Uses {
		if obj.Pkg() != nil && obj.Pkg().Path() == "time" {
			fromTime = append(fromTime, id.Name)
		}
	}
	if want := []string{"Time"}; !slices.Equal(fromTime, want) {
		t.Errorf("uses of package time = %q, want %q", fromTime, want)
	}
}

func TestLoad_ListsTheSourceOfATypeError(t *testing.T) {
	t.Parallel()

	r := &recorder{TB: t}
	pkgtest.Load(r, map[string]string{"p_gen.go": "package p\n\nvar x int = \"s\"\n"})

	// The position in the error leads to the numbered line.
	if !r.fatal || len(r.failures) != 1 ||
		!strings.Contains(r.failures[0], "p_gen.go:3:") || !strings.Contains(r.failures[0], "   3  var x int = \"s\"") {
		t.Errorf("Load() failures = %q, want the error with p_gen.go listed", r.failures)
	}
}
