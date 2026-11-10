package validation_test

import (
	"go/constant"
	"go/token"
	"testing"
	"time"

	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/validation"
)

func TestRejects(t *testing.T) {
	t.Parallel()

	pkg := pkgtest.Load(t, map[string]string{"test.go": `package test

import "time"

type Settings struct {
	Name    string ` + "`validate:\"required,min=2,max=5\"`" + `
	Mode    string ` + "`validate:\"oneof=fast slow\"`" + `
	Code    string ` + "`validate:\"omitempty,pattern=^[A-Z]+$\"`" + `
	Port    int    ` + "`validate:\"gte=1,lte=65535\"`" + `
	Retries int8   ` + "`validate:\"oneof=1 3 5\"`" + `
	Ratio   float64 ` + "`validate:\"gt=0\"`" + `
	Debug   bool   ` + "`validate:\"required\"`" + `
	Other   string
	Confirm string ` + "`validate:\"eqfield=Other\"`" + `
	Level   Level  ` + "`validate:\"required\"`" + `
	Host    Host   ` + "`validate:\"omitempty,min=5\"`" + `
	Timeout time.Duration ` + "`validate:\"gte=1s,lte=1h\"`" + `
	Backoff time.Duration ` + "`validate:\"oneof=0 500ms\"`" + `
}

// Level counts from 0, which its IsZero does not take for unset.
type Level int

func (Level) IsZero() bool { return false }

// Host is unset as "none" too.
type Host string

func (h Host) IsZero() bool { return h == "" || h == "none" }
`})
	structs := scan.Structs([]*packages.Package{pkg})
	fields, diags := validation.NewPlanner(pkg.Fset, nil, nil).Struct(structs[0])
	if len(diags) > 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	values := make(map[string]*validation.Value, len(fields))
	for _, f := range fields {
		values[f.Name] = f.Value
	}

	tests := []struct {
		field string
		value constant.Value
		want  string
	}{
		{field: "Name", value: constant.MakeString(""), want: "blank"},
		{field: "Name", value: constant.MakeString("a"), want: "too_short (count=2)"},
		{field: "Name", value: constant.MakeString("abcdef"), want: "too_long (count=5)"},
		{field: "Name", value: constant.MakeString("あいう"), want: ""},
		{field: "Mode", value: constant.MakeString("fast"), want: ""},
		{field: "Mode", value: constant.MakeString("medium"), want: "inclusion"},
		{field: "Code", value: constant.MakeString(""), want: ""},
		{field: "Code", value: constant.MakeString("ABC"), want: ""},
		{field: "Code", value: constant.MakeString("abc"), want: "invalid"},
		{field: "Port", value: constant.MakeInt64(0), want: "greater_than_or_equal_to (count=1)"},
		{field: "Port", value: constant.MakeInt64(8080), want: ""},
		{field: "Port", value: constant.MakeInt64(70000), want: "less_than_or_equal_to (count=65535)"},
		{field: "Retries", value: constant.MakeInt64(3), want: ""},
		{field: "Retries", value: constant.MakeInt64(4), want: "inclusion"},
		{field: "Ratio", value: constant.MakeFromLiteral("0.5", token.FLOAT, 0), want: ""},
		{field: "Ratio", value: constant.MakeInt64(0), want: "greater_than (count=0)"},
		{field: "Debug", value: constant.MakeBool(false), want: "blank"},
		{field: "Debug", value: constant.MakeBool(true), want: ""},
		{field: "Confirm", value: constant.MakeString("x"), want: ""},
		{field: "Level", value: constant.MakeInt64(0), want: ""},
		{field: "Host", value: constant.MakeString("none"), want: ""},
		// A duration is held in nanoseconds, and reported as it is written.
		{field: "Timeout", value: constant.MakeInt64(int64(time.Minute)), want: ""},
		{field: "Timeout", value: constant.MakeInt64(int64(time.Millisecond)), want: "greater_than_or_equal_to (count=1s)"},
		{field: "Timeout", value: constant.MakeInt64(int64(2 * time.Hour)), want: "less_than_or_equal_to (count=1h0m0s)"},
		{field: "Backoff", value: constant.MakeInt64(int64(500 * time.Millisecond)), want: ""},
		{field: "Backoff", value: constant.MakeInt64(int64(time.Second)), want: "inclusion"},
	}
	for _, tt := range tests {
		got, ok := validation.Rejects(values[tt.field], tt.value)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("Rejects(%s, %s) = %q, %v, want %q", tt.field, tt.value, got, ok, tt.want)
		}
	}
	if got, ok := validation.Rejects(nil, constant.MakeString("")); ok {
		t.Errorf("Rejects(nil) = %q, want nothing", got)
	}
}
