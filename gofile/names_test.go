package gofile_test

import (
	"go/types"
	"testing"

	"github.com/go-kanna/internal/gofile"
)

func newFile() *gofile.File {
	return gofile.New("kanna-test", "example.com/app/user", "user")
}

func TestImport(t *testing.T) {
	t.Parallel()

	f := newFile()
	f.Reserve("log")

	tests := []struct {
		path, name, want string
	}{
		{path: "errors", name: "errors", want: "errors"},
		{path: "example.com/app/errors", name: "errors", want: "errors2"},
		{path: "errors", name: "errors", want: "errors"},
		{path: "example.com/app/user", name: "user", want: ""},
		{path: "example.com/log", name: "log", want: "log2"},
	}
	for _, tt := range tests {
		if got := f.Import(tt.path, tt.name); got != tt.want {
			t.Errorf("Import(%q, %q) = %q, want %q", tt.path, tt.name, got, tt.want)
		}
	}
}

func TestScope_Name(t *testing.T) {
	t.Parallel()

	f := newFile()
	f.Reserve("x")
	f.Import("example.com/v", "v")

	s := f.Scope()
	tests := []struct {
		base, want string
	}{
		{base: "i", want: "i"},
		{base: "i", want: "i2"},
		{base: "v", want: "v2"},
		{base: "x", want: "x2"},
	}
	for _, tt := range tests {
		if got := s.Name(tt.base); got != tt.want {
			t.Errorf("Name(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}

	// Another function reuses the names the first one took.
	if got := f.Scope().Name("i"); got != "i" {
		t.Errorf("Name(%q) in a second scope = %q, want i", "i", got)
	}
}

func TestImport_KeepsClearOfLocals(t *testing.T) {
	t.Parallel()

	f := newFile()
	f.Scope().Name("v")

	// Whichever function imports the package may also be the one using v.
	if got := f.Import("example.com/v", "v"); got != "v2" {
		t.Errorf("Import() = %q, want v2", got)
	}
}

func TestQualifier(t *testing.T) {
	t.Parallel()

	f := newFile()
	self := types.NewPackage("example.com/app/user", "user")
	model := types.NewPackage("example.com/app/model", "model")
	typ := types.NewMap(
		types.NewNamed(types.NewTypeName(0, self, "ID", nil), types.Typ[types.Int], nil),
		types.NewPointer(types.NewNamed(types.NewTypeName(0, model, "User", nil), types.NewStruct(nil, nil), nil)),
	)

	if got, want := types.TypeString(typ, f.Qualifier), "map[ID]*model.User"; got != want {
		t.Errorf("TypeString() = %q, want %q", got, want)
	}
	if got := f.Qualifier(nil); got != "" {
		t.Errorf("Qualifier(nil) = %q, want empty", got)
	}
}
