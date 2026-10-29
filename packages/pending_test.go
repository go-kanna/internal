package packages_test

import (
	"slices"
	"testing"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/packages"
)

func TestSet_Pending(t *testing.T) {
	t.Parallel()

	const q = "package q\n\ntype U struct{}\n"

	tests := []struct {
		name  string
		files map[string]string
		names []string
		want  []string // where the pending errors are
	}{
		{
			name:  "an undefined function",
			files: map[string]string{"p/p.go": "package p\n\nvar _ = F()\n\nvar _ = G()\n"},
			names: []string{"example.com/m/p.F"},
			want:  []string{"p/p.go:3:9"},
		},
		{
			// go/types names a function as the source does.
			name: "a function of an import, renamed or not",
			files: map[string]string{
				"q/q.go": q,
				"p/p.go": "package p\n\nimport (\n\t\"example.com/m/q\"\n\tr \"example.com/m/q\"\n)\n\n" +
					"var _ = q.G()\n\nvar _ = r.G()\n",
			},
			names: []string{"example.com/m/q.G"},
			want:  []string{"p/p.go:8:11", "p/p.go:10:11"},
		},
		{
			// Nothing but the call uses the import.
			name: "a function dot imported",
			files: map[string]string{
				"q/q.go": q,
				"p/p.go": "package p\n\nimport . \"example.com/m/q\"\n\nvar _ = G()\n",
			},
			names: []string{"example.com/m/q.G"},
			want:  []string{"p/p.go:3:8", "p/p.go:5:9"},
		},
		{
			name: "a function called other than as declared",
			files: map[string]string{
				"p/p.go": "package p\n\nfunc H() int { return 0 }\n\nfunc J(int) int { return 0 }\n\n" +
					"var _ = H(1)\n\nvar _ = J(\"s\")\n\nfunc f() { x, y := H(); _, _ = x, y }\n",
			},
			names: []string{"example.com/m/p.H", "example.com/m/p.J"},
			want:  []string{"p/p.go:7:11", "p/p.go:9:11", "p/p.go:11:20"},
		},
		{
			name: "a method",
			files: map[string]string{
				"p/p.go": "package p\n\ntype T struct{}\n\nvar _ = T{}.M()\n\nvar _ = (&T{}).M()\n\nvar _ = T.M\n",
			},
			names: []string{"example.com/m/p.T.M"},
			want:  []string{"p/p.go:5:13", "p/p.go:7:16", "p/p.go:9:11"},
		},
		{
			name: "a method an interface asks for",
			files: map[string]string{
				"p/p.go": "package p\n\nimport \"fmt\"\n\ntype T struct{}\n\n" +
					"var _ fmt.Stringer = T{}\n\nvar _ fmt.Stringer = &T{}\n",
			},
			names: []string{"example.com/m/p.T.String"},
			want:  []string{"p/p.go:7:22", "p/p.go:9:22"},
		},
		{
			// go/types names q.U by its package's name, whatever the file
			// imports it as.
			name: "a method of a type of an import",
			files: map[string]string{
				"q/q.go": q,
				"p/p.go": "package p\n\nimport (\n\t\"fmt\"\n\n\tr \"example.com/m/q\"\n)\n\n" +
					"var _ = r.U{}.M()\n\nvar _ fmt.Stringer = r.U{}\n",
			},
			names: []string{"example.com/m/q.U.M", "example.com/m/q.U.String"},
			want:  []string{"p/p.go:9:15", "p/p.go:11:22"},
		},
		{
			// go/types names each by its path, as both are named q.
			name: "a method of a type of two imports of one name",
			files: map[string]string{
				"q/q.go":   q,
				"x/q/q.go": q,
				"p/p.go": "package p\n\nimport (\n\t\"example.com/m/q\"\n\tq2 \"example.com/m/x/q\"\n)\n\n" +
					"var _ = q.U{}.M()\n\nvar _ = q2.U{}.M()\n",
			},
			names: []string{"example.com/m/x/q.U.M"},
			want:  []string{"p/p.go:10:16"},
		},
		{
			name:  "an error about nothing of names",
			files: map[string]string{"p/p.go": "package p\n\nvar _ int = \"s\"\n\nvar _ = G()\n"},
			names: []string{"example.com/m/p.F"},
		},
		{
			name: "an error of a package that is not a root",
			files: map[string]string{
				"q/q.go": "package q\n\nvar V = G()\n",
				"p/p.go": "package p\n\nimport \"example.com/m/q\"\n\nvar _ = q.V\n",
			},
			names: []string{"example.com/m/q.G"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := module(t, tt.files)
			set := load(t, []string{"./p"}, packages.Config{Dir: dir})
			names := make(map[string]bool, len(tt.names))
			for _, n := range tt.names {
				names[n] = true
			}

			pending := set.Pending(names)
			got := where(t, dir, slices.DeleteFunc(slices.Clone(set.Diags), func(d diag.Diag) bool { return !pending(d) }))
			if !slices.Equal(got, tt.want) {
				t.Errorf("pending at %q, want %q\n%v", got, tt.want, set.Diags)
			}
		})
	}
}
