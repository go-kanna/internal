package packages_test

import (
	"fmt"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/packages"
)

// TestMain loads every module the tests write outside any workspace. A GOWORK
// pinned by the environment would otherwise put them outside the modules it
// lists, which the go command refuses to load.
func TestMain(m *testing.M) {
	// Before any test runs: t.Setenv would rule out t.Parallel.
	if err := os.Setenv("GOWORK", "off"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// module writes files into a new module named example.com/m and returns its
// directory, with symlinks resolved so that it matches the paths the go command
// reports.
func module(t *testing.T, files map[string]string) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, src string) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.26.0\n")
	for name, src := range files {
		write(name, src)
	}
	return dir
}

func load(t *testing.T, patterns []string, cfg packages.Config) packages.Set {
	t.Helper()

	set, err := packages.Load(patterns, cfg)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return set
}

// paths lists the import paths of pkgs, each with its number of Go files.
func paths(pkgs []*packages.Package) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, fmt.Sprintf("%s (%d)", p.PkgPath, len(p.GoFiles)))
	}
	return out
}

// where lists the positions of ds relative to dir, or "-" for none.
func where(t *testing.T, dir string, ds []diag.Diag) []string {
	t.Helper()

	out := make([]string, 0, len(ds))
	for _, d := range ds {
		if d.Pos.Filename == "" {
			out = append(out, "-")
			continue
		}
		rel, err := filepath.Rel(dir, d.Pos.Filename)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%s:%d:%d", filepath.ToSlash(rel), d.Pos.Line, d.Pos.Column))
	}
	return out
}

func TestLoad_OnePackagePerImportPath(t *testing.T) {
	t.Parallel()

	dir := module(t, map[string]string{
		"p/p.go":        "package p\n\ntype T struct{}\n",
		"p/p_test.go":   "package p\n\nimport \"testing\"\n\nfunc TestT(t *testing.T) {}\n",
		"p/ext_test.go": "package p_test\n\nimport \"testing\"\n\nfunc TestU(t *testing.T) {}\n",
	})

	tests := []struct {
		name  string
		tests bool
		want  []string
	}{
		{name: "without tests", tests: false, want: []string{"example.com/m/p (1)"}},
		// The test variant replaces the package, and the external test package
		// stands on its own. The test binary's main package is gone.
		{name: "with tests", tests: true, want: []string{"example.com/m/p (2)", "example.com/m/p_test (1)"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			set := load(t, []string{"./p"}, packages.Config{Dir: dir, Tests: tt.tests})
			if got := paths(set.Packages); !slices.Equal(got, tt.want) {
				t.Errorf("Packages = %q, want %q", got, tt.want)
			}
			if len(set.Diags) > 0 {
				t.Errorf("Diags = %v, want none", set.Diags)
			}
		})
	}
}

func TestLoad_NoPatternsMeansDir(t *testing.T) {
	t.Parallel()

	dir := module(t, map[string]string{"p/p.go": "package p\n"})

	set := load(t, nil, packages.Config{Dir: filepath.Join(dir, "p")})
	if got, want := paths(set.Packages), []string{"example.com/m/p (1)"}; !slices.Equal(got, want) {
		t.Errorf("Packages = %q, want %q", got, want)
	}
}

func TestLoad_ErrorWhenTheGoCommandCannotRun(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := packages.Load(nil, packages.Config{Dir: missing}); err == nil {
		t.Error("Load() in a missing directory succeeded, want an error")
	}
}

func TestLoad_Tags(t *testing.T) {
	t.Parallel()

	dir := module(t, map[string]string{
		"p/p.go":           "package p\n",
		"p/integration.go": "//go:build integration\n\npackage p\n",
	})

	tests := []struct {
		tags string
		want int
	}{
		{tags: "", want: 1},
		{tags: "integration", want: 2},
		{tags: "integration,e2e", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.tags, func(t *testing.T) {
			t.Parallel()

			set := load(t, []string{"./p"}, packages.Config{Dir: dir, Tags: tt.tags})
			if got := len(set.Packages[0].GoFiles); got != tt.want {
				t.Errorf("GoFiles has %d files, want %d", got, tt.want)
			}
		})
	}
}

func TestLoad_TypesInfo(t *testing.T) {
	t.Parallel()

	const src = "package p\n\nimport \"strconv\"\n\nvar _ = []any{strconv.Itoa}\n"
	itoa := []string{"func strconv.Itoa(i int) string"}

	tests := []struct {
		name         string
		files        map[string]string
		cfg          packages.Config
		want         []string
		wantSetAside []string
	}{
		{name: "not asked for", files: map[string]string{"p/p.go": src}},
		{name: "asked for", files: map[string]string{"p/p.go": src}, cfg: packages.Config{TypesInfo: true}, want: itoa},
		{
			// The load that runs again with the stale file set aside asks for it
			// too.
			name: "after setting a file aside",
			files: map[string]string{
				"p/p.go":     src,
				"p/x_gen.go": "// Code generated by kanna-x. DO NOT EDIT.\n\npackage p\n\nfunc F(Missing) {}\n",
			},
			cfg:          packages.Config{TypesInfo: true, Regenerated: []string{"x_gen.go"}},
			want:         itoa,
			wantSetAside: []string{"p/x_gen.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := module(t, tt.files)
			cfg := tt.cfg
			cfg.Dir = dir
			set := load(t, []string{"./p"}, cfg)

			if got := uses(set.Packages[0], "Itoa"); !slices.Equal(got, tt.want) {
				t.Errorf("Itoa refers to %q, want %q", got, tt.want)
			}
			if got := relative(t, dir, set.SetAside); !slices.Equal(got, tt.wantSetAside) {
				t.Errorf("SetAside = %q, want %q", got, tt.wantSetAside)
			}
		})
	}
}

func TestLoad_Deps(t *testing.T) {
	t.Parallel()

	// a imports b alone, whose API refers to c.T; c also declares NewT.
	dir := module(t, map[string]string{
		"a/a.go": "package a\n\nimport \"example.com/m/b\"\n\nvar _ = b.F\n",
		"b/b.go": "package b\n\nimport \"example.com/m/c\"\n\nfunc F(c.T) {}\n",
		"c/c.go": "package c\n\ntype T struct{}\n\nfunc NewT() T { return T{} }\n",
	})

	tests := []struct {
		name string
		deps bool
		want []string
	}{
		// From the export data of b, c is only what b refers to.
		{name: "from export data", want: []string{"T"}},
		{name: "from source", deps: true, want: []string{"NewT", "T"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			set := load(t, []string{"./a"}, packages.Config{Dir: dir, Deps: tt.deps})
			if len(set.Diags) > 0 {
				t.Fatalf("Diags = %v", set.Diags)
			}
			c := imported(imported(set.Packages[0].Types, "example.com/m/b"), "example.com/m/c")
			if c == nil {
				t.Fatal("c is not among what b imports")
			}
			if got := c.Scope().Names(); !slices.Equal(got, tt.want) {
				t.Errorf("c declares %q, want %q", got, tt.want)
			}
		})
	}
}

// A generator goes from a type read in export data to the file declaring it,
// which needs a path on disk whatever GOFLAGS says.
func TestLoad_PathsOnDiskUnderTrimpath(t *testing.T) {
	t.Setenv("GOFLAGS", "-trimpath")

	dir := module(t, map[string]string{
		"a/a.go": "package a\n\nimport \"example.com/m/b\"\n\nvar _ b.T\n",
		"b/b.go": "package b\n\ntype T struct{}\n",
	})

	for _, tags := range []string{"", "integration"} {
		set := load(t, []string{"./a"}, packages.Config{Dir: dir, Tags: tags})
		if len(set.Diags) > 0 {
			t.Fatalf("Diags = %v", set.Diags)
		}
		b := imported(set.Packages[0].Types, "example.com/m/b")
		if b == nil {
			t.Fatal("b is not among what a imports")
		}
		got := set.Fset.Position(b.Scope().Lookup("T").Pos()).Filename
		if want := filepath.Join(dir, "b", "b.go"); got != want {
			t.Errorf("with tags %q, T is declared at %q, want %q", tags, got, want)
		}
	}
}

// imported returns what pkg imports from path, or nil.
func imported(pkg *types.Package, path string) *types.Package {
	if pkg == nil {
		return nil
	}
	for _, imp := range pkg.Imports() {
		if imp.Path() == path {
			return imp
		}
	}
	return nil
}

// uses lists what each use of name in pkg refers to, which needs TypesInfo.
func uses(pkg *packages.Package, name string) []string {
	if pkg.TypesInfo == nil {
		return nil
	}
	var out []string
	for id, obj := range pkg.TypesInfo.Uses {
		if id.Name == name {
			out = append(out, obj.String())
		}
	}
	return out
}

func TestLoad_Overlay(t *testing.T) {
	t.Parallel()

	dir := module(t, map[string]string{"p/p.go": "package p\n\nvar V = 1\n", "p/q.go": "package p\n\nvar W = V\n"})

	set := load(t, []string{"./p"}, packages.Config{
		Dir:     dir,
		Overlay: map[string][]byte{filepath.Join(dir, "p", "p.go"): []byte("package p\n")},
	})

	if got, want := where(t, dir, set.Diags), []string{"p/q.go:3:9"}; !slices.Equal(got, want) {
		t.Errorf("Diags at %q, want %q, as the overlay leaves V undefined\n%v", got, want, set.Diags)
	}
}

func TestSet_Paths(t *testing.T) {
	t.Parallel()

	dir := module(t, map[string]string{"b/b.go": "package b\n", "a/a.go": "package a\n"})

	set := load(t, []string{"./..."}, packages.Config{Dir: dir})

	want := []string{"example.com/m/a", "example.com/m/b"}
	if got := set.Paths(); !slices.Equal(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
}

func TestSet_Paths_ExternalTests(t *testing.T) {
	t.Parallel()

	dir := module(t, map[string]string{"a/a.go": "package a\n", "a/x_test.go": "package a_test\n"})

	set := load(t, []string{"./a"}, packages.Config{Dir: dir, Tests: true})

	want := []string{"example.com/m/a"}
	if got := set.Paths(); !slices.Equal(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
}

func TestLoad_Diags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		pattern string
		tests   bool
		want    []string
	}{
		{
			// The compiler reports both errors and go/types reports them again;
			// each appears once, in source order although go/types reports the
			// unused variable last.
			name:    "type errors",
			files:   map[string]string{"bad/bad.go": "package bad\n\nfunc F() int { var x int; return \"s\" }\n"},
			pattern: "./bad",
			want:    []string{"bad/bad.go:3:20", "bad/bad.go:3:34"},
		},
		{
			// go/parser reports this one mistake four times over, all on one
			// line.
			name:    "syntax error",
			files:   map[string]string{"syntax/syntax.go": "package syntax\n\nfunc F( {\n"},
			pattern: "./syntax",
			want:    []string{"syntax/syntax.go:3:9"},
		},
		{
			// top and mid record no errors of their own.
			name: "broken dependency",
			files: map[string]string{
				"bad/bad.go": "package bad\n\nfunc F() int { var x int; return 1 }\n",
				"mid/mid.go": "package mid\n\nimport \"example.com/m/bad\"\n\nfunc G() int { return bad.F() }\n",
				"top/top.go": "package top\n\nimport \"example.com/m/mid\"\n\nvar V = mid.G()\n",
			},
			pattern: "./top",
			want:    []string{"bad/bad.go:3:20"},
		},
		{
			// The go command reports it too, by a path relative to the
			// directory it ran in.
			name:    "a file that does not parse as far as its package clause",
			files:   map[string]string{"p/p.go": "package p\n", "p/x.go": "<<<<<<< HEAD\n=======\n>>>>>>> f\n\npackage p\n"},
			pattern: "./p",
			want:    []string{"-", "p/x.go:1:1"},
		},
		{
			// b imports a, which comes back as itself and as its test variant.
			name: "a tested package another imports",
			files: map[string]string{
				"a/a.go":      "package a\n\nvar X int = \"s\"\n",
				"a/a_test.go": "package a\n",
				"b/b.go":      "package b\n\nimport \"example.com/m/a\"\n\nvar _ = a.X\n",
			},
			pattern: "./...",
			tests:   true,
			want:    []string{"a/a.go:3:13"},
		},
		{
			name:    "no such directory",
			files:   map[string]string{},
			pattern: "./nothing",
			want:    []string{"-"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := module(t, tt.files)
			set := load(t, []string{tt.pattern}, packages.Config{Dir: dir, Tests: tt.tests})
			if got := where(t, dir, set.Diags); !slices.Equal(got, tt.want) {
				t.Errorf("Diags at %q, want %q\n%v", got, tt.want, set.Diags)
			}
			if !diag.HasErrors(set.Diags) {
				t.Errorf("Diags hold no error: %v", set.Diags)
			}
		})
	}
}

func TestLoad_KeepsTheLinesThatContinueAMessage(t *testing.T) {
	t.Parallel()

	dir := module(t, map[string]string{
		"p/p.go": "package p\n\ntype I interface{ M(int) }\n\ntype T struct{}\n\nfunc (T) M(string) {}\n\nvar _ I = T{}\n",
	})

	set := load(t, []string{"./p"}, packages.Config{Dir: dir})
	if len(set.Diags) != 1 {
		t.Fatalf("Diags = %v, want one", set.Diags)
	}
	// The have/want lines of a mismatch are indented with tabs below the
	// message they belong to.
	if msg := set.Diags[0].Message; !strings.Contains(msg, "\n\t") {
		t.Errorf("Message = %q, want it to keep its continuation lines", msg)
	}
}

func TestGeneratedFiles(t *testing.T) {
	t.Parallel()

	dir := module(t, map[string]string{
		"p/p.go":     "package p\n",
		"p/p_gen.go": "// Code generated by kanna-validate. DO NOT EDIT.\n\npackage p\n",
		"p/q_gen.go": "// Code generated by kanna-validate v0.0.2. DO NOT EDIT.\n\npackage p\n",
		// Neither parses as far as its package clause, and only the first
		// has the marker past its conflict markers.
		"p/r_gen.go": "<<<<<<< HEAD\n// Code generated by kanna-validate. DO NOT EDIT.\n=======\n>>>>>>> f\n\npackage p\n",
		"p/s.go":     "<<<<<<< HEAD\n=======\n>>>>>>> f\n\npackage p\n",
	})

	set := load(t, []string{"./p"}, packages.Config{Dir: dir})
	got := packages.GeneratedFiles(set.Packages[0])

	want := map[string]bool{
		filepath.Join(dir, "p", "p_gen.go"): true,
		filepath.Join(dir, "p", "q_gen.go"): true,
		filepath.Join(dir, "p", "r_gen.go"): true,
	}
	if !maps.Equal(got, want) {
		t.Errorf("GeneratedFiles() = %v, want %v", got, want)
	}
}
