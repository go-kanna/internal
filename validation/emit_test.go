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
// The generated code calls go-kanna/validator, and testdata/format the rules of
// go-kanna/value, neither of which this module requires, so the golden files
// are only compared. kanna-validator compiles and calls what it generates.
func TestEmit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dir    string
		golden string
	}{
		{dir: "testdata/emit", golden: "validator_gen.go.golden"},
		{dir: "testdata/format", golden: "validator_gen.go.golden"},
	}

	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			t.Parallel()

			set, err := packages.Load([]string{"./" + tt.dir}, packages.Config{Regenerated: []string{"validator_gen.go"}})
			if err != nil || len(set.Diags) > 0 {
				t.Fatalf("load: %v %v", err, set.Diags)
			}
			dir, err := filepath.Abs(tt.dir)
			if err != nil {
				t.Fatal(err)
			}
			declared := slices.Sorted(maps.Keys(output.DeclaredNames(dir, "validator_gen.go", set.Packages[0].Name)))

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

	declared := []string{"x", "errs", "i", "validator", "patternTargetQuote"}
	got, err := validation.Emit(pkg.Fset, []*packages.Package{pkg}, declared)
	if err != nil {
		t.Fatalf("emit: %v\n%s", err, got)
	}

	for _, want := range []string{
		"func (x2 Target) Validate() error {",
		"var errs2 validator2.Errors",
		"for i2 := range x2.Tags {",
		"patternTargetQuote2 = regexp.MustCompile(\"a`b\")",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
}

// TestEmit_ValueAt checks that Body.ValueAt writes the checks of Body.Value
// under the paths it is given, of more than one segment.
func TestEmit_ValueAt(t *testing.T) {
	t.Parallel()

	set, err := packages.Load([]string{"./testdata/emit"}, packages.Config{Regenerated: []string{"validator_gen.go"}})
	if err != nil || len(set.Diags) > 0 {
		t.Fatalf("load: %v %v", err, set.Diags)
	}
	dir, err := filepath.Abs("testdata/emit")
	if err != nil {
		t.Fatal(err)
	}
	declared := slices.Sorted(maps.Keys(output.DeclaredNames(dir, "validator_gen.go", set.Packages[0].Name)))

	want, err := validation.Emit(set.Fset, set.Packages, declared)
	if err != nil {
		t.Fatalf("emit: %v\n%s", err, want)
	}
	got, err := validation.EmitAt(set.Fset, set.Packages, declared, func(rt, name, nestName string) (string, string) {
		nestPath := rt + ".Path{}"
		if nestName != "" {
			nestPath = rt + `.Field("section", ` + nestName + ")"
		}
		return rt + `.Field("section", ` + name + ")", nestPath
	})
	if err != nil {
		t.Fatalf("emit: %v\n%s", err, got)
	}

	if w := strings.ReplaceAll(string(want), "validator.Field(", `validator.Field("section", `); string(got) != w {
		t.Errorf("ValueAt wrote\n%s\nwant\n%s", got, w)
	}
	if !strings.Contains(string(got), `validator.Field("section", `) {
		t.Error("ValueAt wrote no path of two segments")
	}
}
