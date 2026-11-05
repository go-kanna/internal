package cli_test

import (
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/cli"
	"github.com/go-kanna/internal/diag"
)

type vetRun = func(dir, tags string, patterns []string) []diag.Diag

func vet(run vetRun) cli.Vet {
	return cli.Vet{Name: "kanna-test", Summary: "find what tests need", Run: run}
}

func vetIn(t *testing.T, v cli.Vet, dir string, args ...string) result {
	t.Helper()

	var stdout, stderr strings.Builder
	code := v.Exec(cli.Env{Out: &stdout, Err: &stderr, Dir: dir}, args)
	return result{code: code, stdout: stdout.String(), err: stderr.String()}
}

func TestVet_PassesFlagsAndPackages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	type call struct {
		dir, tags string
		patterns  []string
	}
	var calls []call
	v := vet(func(dir, tags string, patterns []string) []diag.Diag {
		calls = append(calls, call{dir: dir, tags: tags, patterns: patterns})
		return nil
	})

	vetIn(t, v, dir, "-tags", "a,b", "./model", "help")
	vetIn(t, v, dir)

	if len(calls) != 2 {
		t.Fatalf("Run called %d times, want 2", len(calls))
	}
	// A "help" that does not lead is a package like any other.
	if got := calls[0]; got.dir != dir || got.tags != "a,b" || !slices.Equal(got.patterns, []string{"./model", "help"}) {
		t.Errorf("first run = %+v", got)
	}
	if got := calls[1]; got.tags != "" || !slices.Equal(got.patterns, []string{"."}) {
		t.Errorf("second run = %+v, want no tags and the package in the directory", got)
	}
}

func TestVet_WorkingDirectory(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var got string
	vetIn(t, vet(func(dir, _ string, _ []string) []diag.Diag {
		got = dir
		return nil
	}), "")

	if got != wd {
		t.Errorf("Run got dir %q, want the working directory %q", got, wd)
	}
}

func TestVet_Diagnostics(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	at := token.Position{Filename: filepath.Join(dir, "p", "p.go"), Line: 3, Column: 7}
	file := "p" + string(filepath.Separator) + "p.go:3:7"

	tests := []struct {
		name string
		ds   []diag.Diag
		want result
	}{
		{name: "nothing", want: result{code: cli.ExitOK}},
		{
			name: "a warning",
			ds:   []diag.Diag{diag.Warningf(at, "odd")},
			want: result{code: cli.ExitOK, err: file + ": warning: odd\n"},
		},
		{
			name: "an error",
			ds:   []diag.Diag{diag.Errorf(at, "wrong").WithHints("do this")},
			want: result{code: cli.ExitFailure, err: file + ": error: wrong\n  do this\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := vetIn(t, vet(func(string, string, []string) []diag.Diag { return tt.ds }), dir)
			if got != tt.want {
				t.Errorf("Exec() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestVet_UsageAndVersion(t *testing.T) {
	t.Parallel()

	usage := "kanna-test vet: find what tests need\n\nUsage:\n  kanna-test vet [flags] [packages]\n\n" +
		"The packages default to the one in the current directory. A package passing holds nothing\n" +
		"vet found; it does not prove there is nothing to find.\n\nFlags:\n"

	tests := []struct {
		name    string
		args    []string
		want    int
		wantOut string // a prefix of stdout
		wantErr string // a prefix of stderr
	}{
		{name: "help", args: []string{"help"}, want: cli.ExitOK, wantOut: usage},
		{name: "-h", args: []string{"-h"}, want: cli.ExitOK, wantOut: usage},
		{
			name:    "unknown flag",
			args:    []string{"-nope"},
			want:    cli.ExitUsage,
			wantErr: "flag provided but not defined: -nope\n" + usage,
		},
		{name: "-version", args: []string{"-version"}, want: cli.ExitOK, wantOut: "kanna-test "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ran := false
			got := vetIn(t, vet(func(string, string, []string) []diag.Diag {
				ran = true
				return nil
			}), t.TempDir(), tt.args...)

			if got.code != tt.want || !strings.HasPrefix(got.stdout, tt.wantOut) || !strings.HasPrefix(got.err, tt.wantErr) {
				t.Errorf("Exec(%q) = %+v, want code %d, stdout %q..., stderr %q...",
					tt.args, got, tt.want, tt.wantOut, tt.wantErr)
			}
			if (tt.wantOut == "") != (got.stdout == "") || (tt.wantErr == "") != (got.err == "") {
				t.Errorf("Exec(%q) wrote to the wrong stream: %+v", tt.args, got)
			}
			if ran {
				t.Errorf("Exec(%q) ran the vet", tt.args)
			}
		})
	}

	if got := vetIn(t, vet(nil), t.TempDir(), "-h"); !strings.Contains(got.stdout, "  -tags tags\n") ||
		!strings.Contains(got.stdout, "  -version\n") {
		t.Errorf("usage lists no -tags or no -version:\n%s", got.stdout)
	}
}
