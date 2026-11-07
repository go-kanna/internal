package validation_test

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/output"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"
	"github.com/go-kanna/internal/validation"
)

// TestEmit compares the output for each testdata package with its golden file.
// To accept a change in the output:
//
//	go test ./validation -run TestEmit -update
//
// The generated code calls go-kanna/validate, and testdata/format the rules of
// go-kanna/value, neither of which this module requires, so the golden files
// are only compared. kanna-validate compiles and calls what it generates.
func TestEmit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dir    string
		golden string
	}{
		{dir: "testdata/emit", golden: "validate_gen.go.golden"},
		{dir: "testdata/format", golden: "validate_gen.go.golden"},
	}

	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			t.Parallel()

			set, err := packages.Load([]string{"./" + tt.dir}, packages.Config{Regenerated: []string{"validate_gen.go"}})
			if err != nil || len(set.Diags) > 0 {
				t.Fatalf("load: %v %v", err, set.Diags)
			}
			dir, err := filepath.Abs(tt.dir)
			if err != nil {
				t.Fatal(err)
			}
			declared := slices.Sorted(maps.Keys(output.DeclaredNames(dir, "validate_gen.go", set.Packages[0].Name)))

			got, err := validation.Emit(set.Fset, set.Packages, declared)
			if err != nil {
				t.Fatalf("emit: %v\n%s", err, got)
			}
			pkgtest.Golden(t, filepath.Join(tt.dir, tt.golden), got)
		})
	}
}

func TestEmit_KeepsClearOfDeclaredNames(t *testing.T) {
	t.Parallel()

	pkg := pkgtest.Load(t, map[string]string{"test.go": `package test

//kanna:validate
type Target struct {
	Tags  []string ` + "`validate:\"dive,max=3\"`" + `
	Quote string   "validate:\"pattern=a` + "`" + `b\""
}
`})

	declared := []string{"x", "errs", "i", "validate", "patternTargetQuote"}
	got, err := validation.Emit(pkg.Fset, []*packages.Package{pkg}, declared)
	if err != nil {
		t.Fatalf("emit: %v\n%s", err, got)
	}

	for _, want := range []string{
		"func (x2 Target) Validate() error {",
		"var errs2 validate2.Errors",
		"for i2 := range x2.Tags {",
		"patternTargetQuote2 = regexp.MustCompile(\"a`b\")",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
}
