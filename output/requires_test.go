package output_test

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-kanna/internal/output"
)

// TestMissingRequires sets GOWORK, so that whatever the person running it has
// set does not decide the outcome, and so cannot run in parallel.
func TestMissingRequires(t *testing.T) {
	const gomod = "module example.com/app\n\ngo 1.26.0\n\nrequire github.com/go-kanna/validated v0.0.2\n"
	imports := []string{"example.com/app/model", "github.com/go-kanna/validated/rules"}

	tests := []struct {
		name    string
		files   map[string]string
		gowork  string
		imports []string
		want    []string
	}{
		{
			// The module's own packages and those under a requirement are provided.
			name:    "everything provided",
			files:   map[string]string{"go.mod": gomod},
			imports: imports,
		},
		{
			name:    "a module not required",
			files:   map[string]string{"go.mod": gomod},
			imports: append(slices.Clone(imports), "github.com/brianvoe/gofakeit/v7", "github.com/google/uuid"),
			want: []string{
				"imports github.com/brianvoe/gofakeit/v7, github.com/google/uuid, which go.mod does not require; " +
					"run go mod tidy",
			},
		},
		{
			// A module path is a prefix of its packages only up to a slash.
			name:    "a path sharing a requirement's prefix",
			files:   map[string]string{"go.mod": gomod},
			imports: []string{"github.com/go-kanna/validatedx"},
			want:    []string{"imports github.com/go-kanna/validatedx, which go.mod does not require; run go mod tidy"},
		},
		{name: "no go.mod", imports: []string{"github.com/google/uuid"}},
		{
			name:    "a go.mod that does not parse",
			files:   map[string]string{"go.mod": "module\n"},
			imports: []string{"github.com/google/uuid"},
		},
		{
			// The workspace may provide it.
			name:    "in a workspace",
			files:   map[string]string{"go.mod": gomod, "../go.work": "go 1.26.0\n\nuse ./app\n"},
			imports: []string{"github.com/google/uuid"},
		},
		{
			name:    "in a workspace named by GOWORK",
			files:   map[string]string{"go.mod": gomod},
			gowork:  "/elsewhere/go.work",
			imports: []string{"github.com/google/uuid"},
		},
		{
			name:    "with workspaces turned off",
			files:   map[string]string{"go.mod": gomod, "../go.work": "go 1.26.0\n\nuse ./app\n"},
			gowork:  "off",
			imports: []string{"github.com/google/uuid"},
			want:    []string{"imports github.com/google/uuid, which go.mod does not require; run go mod tidy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOWORK", tt.gowork)

			module := filepath.Join(t.TempDir(), "app")
			files := map[string]string{"fixture/.keep": ""}
			maps.Copy(files, tt.files)
			put(t, module, files)
			// The file is generated a directory below go.mod.
			path := filepath.Join(module, "fixture", "fixture_gen.go")

			ds := output.MissingRequires(path, tt.imports)
			if got := messages(ds); !slices.Equal(got, tt.want) {
				t.Errorf("MissingRequires() = %q, want %q", got, tt.want)
			}
			for _, d := range ds {
				if d.Pos.Filename != path {
					t.Errorf("warning at %q, want %q", d.Pos.Filename, path)
				}
			}
		})
	}
}
