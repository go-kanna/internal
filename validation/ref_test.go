package validation_test

import (
	"slices"
	"testing"

	"github.com/go-kanna/internal/deps"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/validation"
)

// RuleRefs names a package by how the struct's file imports it: by the name of
// a named import, or by the package's own name where it is imported as _.
func TestRuleRefs(t *testing.T) {
	t.Parallel()

	pkg := pkgtest.Load(t, map[string]string{"test.go": `package test

import (
	"time"

	_ "net/url"
)

type Target struct {
	When  time.Time   ` + "`validate:\"rule=time.Time\"`" + `
	Whens []time.Time ` + "`validate:\"dive,rule=time.Time\"`" + `
	Link  string      ` + "`validate:\"rule=url.URL\"`" + `
	Own   string      ` + "`validate:\"rule=Local\"`" + `
	Lost  string      ` + "`validate:\"rule=nowhere.Type\"`" + `
	Bad   string      ` + "`validate:\"rule=a..b\"`" + `
}
`})
	structs := scan.Structs([]*packages.Package{pkg})
	if len(structs) != 1 {
		t.Fatalf("structs = %d, want 1", len(structs))
	}

	want := []deps.Ref{
		{Package: "time", Via: "rule=time.Time"},
		{Package: "time", Via: "rule=time.Time"},
		{Package: "net/url", Via: "rule=url.URL"},
	}
	if got := validation.RuleRefs(structs[0]); !slices.Equal(got, want) {
		t.Errorf("RuleRefs = %+v, want %+v", got, want)
	}
}
