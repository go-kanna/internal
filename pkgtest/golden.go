package pkgtest

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files from what the code now generates")

// Golden compares got, the output of a generator, with the golden file at
// path. Run with -update, it writes got there instead.
//
// A golden file that is Go source is best kept as a file of a testdata
// package that the test imports: then a golden that stops compiling fails the
// build of the test, and the test can call what was generated.
func Golden(t testing.TB, path string, got []byte) {
	t.Helper()

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		// Golden files are source, read like the rest of the tree.
		//nolint:gosec // see above
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s does not exist; run go test -update to create it", path)
		return
	}
	if err != nil {
		t.Fatal(err)
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from what the code generates; run go test -update to accept it\n%s",
			path, firstDifference(string(want), string(got)))
	}
}

// firstDifference shows the first line where want and got part ways.
func firstDifference(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := range max(len(wantLines), len(gotLines)) {
		w, g := line(wantLines, i), line(gotLines, i)
		if w != g {
			return fmt.Sprintf("line %d:\n  golden:    %s\n  generated: %s", i+1, w, g)
		}
	}
	return "they differ only in how their lines end"
}

func line(lines []string, i int) string {
	if i < len(lines) {
		return strings.TrimSuffix(lines[i], "\r")
	}
	return "(end of file)"
}
