package packages_test

import (
	"go/token"
	"path/filepath"
	"testing"

	"github.com/go-kanna/internal/packages"
)

func TestPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want token.Position
	}{
		{in: "", want: token.Position{}},
		{in: "-", want: token.Position{}},
		{in: "a.go", want: token.Position{Filename: "a.go"}},
		{in: "a.go:3", want: token.Position{Filename: "a.go", Line: 3}},
		{in: "a.go:3:4", want: token.Position{Filename: "a.go", Line: 3, Column: 4}},
		{in: "a.go:x", want: token.Position{Filename: "a.go:x"}},
		{in: `C:\src\a.go:3:4`, want: token.Position{Filename: `C:\src\a.go`, Line: 3, Column: 4}},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := packages.Position(tt.in); got != tt.want {
				t.Errorf("position(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCompilerDiags(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(string(filepath.Separator), "src")
	out := "# example.com/m/p\n" +
		"./a.go:1:2: first\n" +
		"\t\thave M(string)\n" +
		"note: module requires Go 1.99\n" +
		"sub/b.go:3: second\n"

	got := packages.CompilerDiags(out, dir)

	want := []struct {
		pos token.Position
		msg string
	}{
		{pos: token.Position{Filename: filepath.Join(dir, "a.go"), Line: 1, Column: 2}, msg: "first\n\t\thave M(string)"},
		{pos: token.Position{}, msg: "note: module requires Go 1.99"},
		{pos: token.Position{Filename: filepath.Join(dir, "sub", "b.go"), Line: 3}, msg: "second"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d diagnostics, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Pos != w.pos || got[i].Message != w.msg {
			t.Errorf("diagnostic %d = %v %q, want %v %q", i, got[i].Pos, got[i].Message, w.pos, w.msg)
		}
	}
}
