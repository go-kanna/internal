package diag_test

import (
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kanna/internal/diag"
)

func TestSeverity_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		s    diag.Severity
		want string
	}{
		{s: diag.Error, want: "error"},
		{s: diag.Warning, want: "warning"},
		{s: diag.Severity(7), want: "Severity(7)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			if got := tt.s.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiag_String(t *testing.T) {
	t.Parallel()

	pos := token.Position{Filename: "user.go", Line: 12, Column: 3}

	tests := []struct {
		name string
		d    diag.Diag
		want string
	}{
		{
			name: "error at a position",
			d:    diag.Errorf(pos, "unknown rule %q", "emial"),
			want: `user.go:12:3: error: unknown rule "emial"`,
		},
		{
			name: "warning",
			d:    diag.Warningf(pos, "unused"),
			want: "user.go:12:3: warning: unused",
		},
		{
			name: "no position",
			d:    diag.Errorf(token.Position{}, "no package to generate for"),
			want: "error: no package to generate for",
		},
		{
			name: "hints",
			d:    diag.Errorf(pos, "unknown rule").WithHints("did you mean email?", "see the README"),
			want: "user.go:12:3: error: unknown rule\n  did you mean email?\n  see the README",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.d.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiag_WithHintsLeavesTheBaseAlone(t *testing.T) {
	t.Parallel()

	base := diag.Errorf(token.Position{}, "m")
	// Spare capacity is what would let two appends write into one array.
	base.Hints = append(make([]string, 0, 4), "shared")

	a := base.WithHints("a")
	b := base.WithHints("b")

	if got := strings.Join(a.Hints, ","); got != "shared,a" {
		t.Errorf("a.Hints = %q, want shared,a", got)
	}
	if got := strings.Join(b.Hints, ","); got != "shared,b" {
		t.Errorf("b.Hints = %q, want shared,b", got)
	}
	if got := strings.Join(base.Hints, ","); got != "shared" {
		t.Errorf("base.Hints = %q, want shared", got)
	}
}

func TestHasErrors(t *testing.T) {
	t.Parallel()

	warning := diag.Warningf(token.Position{}, "w")
	err := diag.Errorf(token.Position{}, "e")

	tests := []struct {
		name string
		ds   []diag.Diag
		want bool
	}{
		{name: "none", ds: nil, want: false},
		{name: "warnings only", ds: []diag.Diag{warning, warning}, want: false},
		{name: "an error among warnings", ds: []diag.Diag{warning, err}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := diag.HasErrors(tt.ds); got != tt.want {
				t.Errorf("HasErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "app")
	far := filepath.Join(filepath.VolumeName(root)+string(filepath.Separator), "x.go")

	at := func(path string) token.Position {
		return token.Position{Filename: path, Line: 1, Column: 2}
	}

	tests := []struct {
		name string
		dir  string
		pos  token.Position
		want string
	}{
		{
			name: "file in the directory",
			dir:  dir,
			pos:  at(filepath.Join(dir, "user.go")),
			want: filepath.FromSlash("./user.go") + ":1:2: error: m\n",
		},
		{
			name: "file below the directory",
			dir:  dir,
			pos:  at(filepath.Join(dir, "sub", "user.go")),
			want: filepath.FromSlash("sub/user.go") + ":1:2: error: m\n",
		},
		{
			name: "file beside the directory",
			dir:  dir,
			pos:  at(filepath.Join(root, "other", "x.go")),
			want: filepath.FromSlash("../other/x.go") + ":1:2: error: m\n",
		},
		{
			name: "file whose absolute path is shorter",
			dir:  dir,
			pos:  at(far),
			want: far + ":1:2: error: m\n",
		},
		{
			name: "no directory",
			dir:  "",
			pos:  at(filepath.Join(dir, "user.go")),
			want: filepath.Join(dir, "user.go") + ":1:2: error: m\n",
		},
		{
			name: "relative path",
			dir:  dir,
			pos:  at("user.go"),
			want: "user.go:1:2: error: m\n",
		},
		{
			name: "no position",
			dir:  dir,
			pos:  token.Position{},
			want: "error: m\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var b strings.Builder
			diag.Write(&b, tt.dir, []diag.Diag{diag.Errorf(tt.pos, "m")})
			if got := b.String(); got != tt.want {
				t.Errorf("Write() wrote %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrite_SeveralWithHints(t *testing.T) {
	t.Parallel()

	ds := []diag.Diag{
		diag.Errorf(token.Position{}, "first").WithHints("hint"),
		diag.Warningf(token.Position{}, "second"),
	}

	var b strings.Builder
	diag.Write(&b, "", ds)

	want := "error: first\n  hint\nwarning: second\n"
	if got := b.String(); got != want {
		t.Errorf("Write() wrote %q, want %q", got, want)
	}
}
