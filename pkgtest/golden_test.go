package pkgtest_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kanna/internal/pkgtest"
)

// recorder is a testing.TB that records failures instead of failing.
type recorder struct {
	testing.TB

	failures []string
	fatal    bool
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.fatal = true
	r.Errorf(format, args...)
}

func (r *recorder) Fatal(args ...any) {
	r.fatal = true
	r.failures = append(r.failures, fmt.Sprint(args...))
}

func TestGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		golden    string // "" for no file
		got       string
		want      string // a fragment of the failure; "" for none
		wantFatal bool
	}{
		{name: "match", golden: "a\nb\n", got: "a\nb\n"},
		{name: "a line differs", golden: "a\nb\nc\n", got: "a\nb\nx\n", want: "line 3:\n  golden:    c\n  generated: x"},
		{name: "a line more", golden: "a\n", got: "a\nb\n", want: "line 2:\n  golden:    \n  generated: b"},
		{name: "a line fewer", golden: "a\nb", got: "a", want: "line 2:\n  golden:    b\n  generated: (end of file)"},
		{name: "line endings", golden: "a\r\nb\r\n", got: "a\nb\n", want: "they differ only in how their lines end"},
		{name: "no golden file", got: "a\n", want: "does not exist; run go test -update to create it", wantFatal: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "p_gen.go.golden")
			if tt.golden != "" {
				if err := os.WriteFile(path, []byte(tt.golden), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			r := &recorder{TB: t}
			pkgtest.Golden(r, path, []byte(tt.got))

			if tt.want == "" {
				if len(r.failures) > 0 {
					t.Errorf("Golden() failed: %q", r.failures)
				}
				return
			}
			if len(r.failures) != 1 || !strings.Contains(r.failures[0], tt.want) || r.fatal != tt.wantFatal {
				t.Errorf("Golden() failures = %q (fatal %v), want one with %q (fatal %v)",
					r.failures, r.fatal, tt.want, tt.wantFatal)
			}
		})
	}
}

// TestGolden_Update sets the -update flag, which every test in the package
// reads, so it cannot run alongside them.
//
//nolint:paralleltest // see above
func TestGolden_Update(t *testing.T) {
	if err := flag.Set("update", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := flag.Set("update", "false"); err != nil {
			t.Error(err)
		}
	})

	path := filepath.Join(t.TempDir(), "testdata", "p_gen.go.golden")
	r := &recorder{TB: t}
	pkgtest.Golden(r, path, []byte("generated\n"))

	if len(r.failures) > 0 {
		t.Errorf("Golden() with -update failed: %q", r.failures)
	}
	if got, err := os.ReadFile(filepath.Clean(path)); err != nil || string(got) != "generated\n" {
		t.Errorf("golden file holds %q, %v, want what was generated", got, err)
	}
}
