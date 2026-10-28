package output_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-kanna/internal/output"
)

func TestPackageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dir     string
		files   map[string]string // nil leaves the directory missing
		want    string
		result  string
		wantErr bool
	}{
		{
			name:   "declared package wins over the directory name",
			dir:    "fixtures",
			files:  map[string]string{"employee.go": "package fixture\n"},
			result: "fixture",
		},
		{
			name:   "wanted name agreeing with the declared one",
			dir:    "fixture",
			files:  map[string]string{"fixture.go": "package fixture\n"},
			want:   "fixture",
			result: "fixture",
		},
		{
			// It would be a file that cannot compile next to the one there.
			name:    "wanted name disagreeing with the declared one",
			dir:     "fixture",
			files:   map[string]string{"fixture.go": "package fixture\n"},
			want:    "fx",
			wantErr: true,
		},
		{name: "wanted name in a missing directory", dir: "fixture", want: "fx", result: "fx"},
		{name: "missing directory", dir: "fixture", result: "fixture"},
		{
			name:   "a previously generated file does not pin the name",
			dir:    "fixture",
			files:  map[string]string{"fixture_gen.go": "package stale\n"},
			result: "fixture",
		},
		{
			name:   "an ignored helper does not count",
			dir:    "fixture",
			files:  map[string]string{"gen.go": "//go:build ignore\n\npackage main\n"},
			result: "fixture",
		},
		{
			// The external test says which package it is for.
			name:   "only an external test",
			dir:    "tests",
			files:  map[string]string{"x_test.go": "package fixture_test\n"},
			result: "fixture",
		},
		{
			name:    "two packages in one directory",
			dir:     "fixture",
			files:   map[string]string{"a.go": "package one\n", "b.go": "package two\n"},
			wantErr: true,
		},
		{name: "directory name that is no identifier", dir: "my-fixture", wantErr: true},
		{name: "wanted name for such a directory", dir: "my-fixture", want: "fixture", result: "fixture"},
		{name: "wanted name that is no identifier", dir: "fixture", want: "1fixture", wantErr: true},
		// An identifier to token.IsIdentifier, but no package can be called so.
		{name: "wanted name that is the blank identifier", dir: "fixture", want: "_", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), tt.dir)
			if tt.files != nil {
				put(t, dir, tt.files)
			}

			got, err := output.PackageName(dir, "fixture_gen.go", tt.want)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("PackageName() = %q, want an error", got)
				}
				return
			}
			if err != nil || got != tt.result {
				t.Errorf("PackageName() = %q, %v, want %q", got, err, tt.result)
			}
		})
	}
}

// The caller reports the error at the directory, so the message does not name
// it again: a file is named relative to it.
func TestPackageName_ErrorsLeaveTheDirectoryOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string]string
		want  string // the wanted package name
		err   string
	}{
		{
			name:  "disagreement",
			files: map[string]string{"fixture.go": "package fixture\n"},
			want:  "fx",
			err:   "the Go files there declare package fixture, not fx",
		},
		{
			name:  "two packages",
			files: map[string]string{"a.go": "package one\n", "b.go": "package two\n"},
			err:   "the Go files there declare two packages: one (a.go) and two (b.go)",
		},
		{
			name:  "a file that does not parse",
			files: map[string]string{"a.go": "hello\n"},
			err:   "cannot tell the package: a.go:1:1: expected 'package', found hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "fixture")
			put(t, dir, tt.files)

			_, err := output.PackageName(dir, "fixture_gen.go", tt.want)
			if err == nil || err.Error() != tt.err {
				t.Errorf("PackageName() error = %v, want %q", err, tt.err)
			}
		})
	}
}

func TestPackageName_UnreadableDirectory(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root is not bound by directory permissions")
	}

	// Falling back to the directory's name would hide what the files declare.
	dir := filepath.Join(t.TempDir(), "fixture")
	put(t, dir, map[string]string{"fixture.go": "package fx\n"})
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	// Give back what put created the directory with, or it cannot be removed.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) }) //nolint:gosec // see above

	if got, err := output.PackageName(dir, "fixture_gen.go", ""); err == nil {
		t.Errorf("PackageName() = %q, want an error", got)
	}
}

func TestDeclaredNames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	put(t, dir, map[string]string{
		"a.go": `package p

func F() {}

type T struct{}

func (T) Method() {}

var V, _ = 1, 2

const C = 3

var (
	A = 1
	B = 2
)
`,
		// A name that exists only on one platform still exists there.
		"a_windows.go": "package p\n\nfunc Win() {}\n",
		"a_test.go":    "package p\n\nfunc helper() {}\n",
		"x_test.go":    "package p_test\n\nfunc External() {}\n",
		"p_gen.go":     "// Code generated by kanna-test. DO NOT EDIT.\n\npackage p\n\nfunc Generated() {}\n",
		"notes.txt":    "func NotGo() {}\n",
		"sub/sub.go":   "package p\n\nfunc Sub() {}\n",
	})

	got := output.DeclaredNames(dir, "p_gen.go", "p")

	want := []string{"A", "B", "C", "F", "T", "V", "Win", "helper"}
	if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, want) {
		t.Errorf("DeclaredNames() = %q, want %q", names, want)
	}
	if pos := got["F"]; pos.Filename != filepath.Join(dir, "a.go") || pos.Line != 3 {
		t.Errorf("F declared at %v, want a.go:3", pos)
	}
	if got := output.DeclaredNames(filepath.Join(dir, "missing"), "p_gen.go", "p"); len(got) > 0 {
		t.Errorf("DeclaredNames() of a missing directory = %v, want none", got)
	}
}

func TestSameDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "model")
	other := filepath.Join(root, "fixture")
	link := filepath.Join(root, "link")
	for _, d := range []string{dir, other} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "same path", a: dir, b: dir, want: true},
		{name: "another spelling", a: dir, b: filepath.Join(root, ".", "fixture", "..", "model"), want: true},
		{name: "through a symlink", a: link, b: dir, want: true},
		{name: "different directories", a: dir, b: other, want: false},
		{name: "a missing directory", a: dir, b: filepath.Join(root, "missing"), want: false},
	}
	for _, tt := range tests {
		if got := output.SameDir(tt.a, tt.b); got != tt.want {
			t.Errorf("%s: SameDir() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
