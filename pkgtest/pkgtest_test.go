package pkgtest_test

import (
	"slices"
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
}
