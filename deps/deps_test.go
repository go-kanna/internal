package deps_test

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-kanna/internal/deps"
)

func TestReport_Sort(t *testing.T) {
	t.Parallel()

	r := deps.Report{
		Packages: []string{"b", "a", "b"},
		Inputs:   []string{"/y", "/x"},
		Writes:   []string{"/q", "/q", "/p"},
		Declares: []string{"p.G", "p.F"},
		Refs: []deps.Ref{
			{Package: "q", Via: "with=q.F"},
			{Package: "p", Via: "with=p.G"},
			{Package: "p", Via: "with=p.F"},
			{Package: "q", Via: "with=q.F"},
		},
	}
	r.Sort()

	want := deps.Report{
		Packages: []string{"a", "b"},
		Inputs:   []string{"/x", "/y"},
		Writes:   []string{"/p", "/q"},
		Declares: []string{"p.F", "p.G"},
		Refs: []deps.Ref{
			{Package: "p", Via: "with=p.F"},
			{Package: "p", Via: "with=p.G"},
			{Package: "q", Via: "with=q.F"},
		},
	}
	if !reflect.DeepEqual(r, want) {
		t.Errorf("Sort() = %+v, want %+v", r, want)
	}
}

func TestReadPending(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "pending")
	if err := os.WriteFile(path, []byte("example.com/p.F\n\nexample.com/p.T.M\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := deps.ReadPending(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"example.com/p.F": true, "example.com/p.T.M": true}
	if !maps.Equal(got, want) {
		t.Errorf("ReadPending() = %v, want %v", got, want)
	}
}

func TestReadPending_NoPath(t *testing.T) {
	t.Parallel()

	got, err := deps.ReadPending("")
	if err != nil || got != nil {
		t.Errorf("ReadPending(\"\") = %v, %v, want none", got, err)
	}
}

func TestReadPending_MissingFile(t *testing.T) {
	t.Parallel()

	if _, err := deps.ReadPending(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Error("ReadPending() of no file reported no error")
	}
}
