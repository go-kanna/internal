// Package deps is how the kanna command and a generator talk: what a generator
// answers to -deps, which is all the command knows of a run before it starts,
// and the names the command passes on to a run for what is still to come.
//
// A generator and the command are released apart, so what travels between them
// is JSON and an environment variable, not a call. A change they cannot both
// read takes a new Protocol.
package deps

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"
)

// Protocol is the version of Report a generator built from this package
// answers with.
const Protocol = 1

// PendingEnv names the environment variable holding the path of a file of
// names, one on each line, as packages.Set.Pending takes them: what runs still
// to come in the same kanna gen are to write. It is how two generators get
// through a package whose code calls what both write, which neither order
// compiles: the first takes those names as coming, and the last finds them
// there, or reports them.
const PendingEnv = "KANNA_PENDING"

// Report is what a run with the same flags would read and write, worked out
// without generating. The paths of Writes and Inputs are absolute.
//
// It is what can be told, not a promise that the run succeeds: a package that
// does not compile yet still gets an answer, as one always does before the
// runs that write what it calls.
type Report struct {
	// Protocol is set by the cli package.
	Protocol int `json:"protocol"`

	// Packages holds the import paths of the packages the run loads itself.
	// What they import, the run reads too.
	Packages []string `json:"packages,omitempty"`

	// Tests says the run loads the tests of Packages, and so what those import.
	Tests bool `json:"tests,omitzero"`

	// SourcesOnly says that of Packages, the run reads what is written by hand
	// alone, so what another generator writes there is none of its input.
	SourcesOnly bool `json:"sourcesOnly,omitzero"`

	// Refs holds the packages the run reads for no import saying so.
	Refs []Ref `json:"refs,omitempty"`

	// Inputs holds the files or directories the run reads that are not Go code.
	Inputs []string `json:"inputs,omitempty"`

	// Writes holds the files the run generates. A file it would only remove is
	// not among them: it is nothing that another run reads.
	Writes []string `json:"writes,omitempty"`

	// Declares names what Writes declare, as packages.Set.Pending takes them.
	Declares []string `json:"declares,omitempty"`
}

// Ref is a package a run reads because something names it.
type Ref struct {
	// Package is the package as it is named: by import path, by the end of
	// one, or by its name, which is all there is to tell one that holds
	// nothing but generated code before it is generated.
	Package string `json:"package"`

	// Via says what names it, as written, such as with=postgres.NewTodo.
	Via string `json:"via"`
}

// Sort puts r in the one order it is answered in, whatever order it was found
// in, each thing once.
func (r *Report) Sort() {
	for _, s := range []*[]string{&r.Packages, &r.Inputs, &r.Writes, &r.Declares} {
		slices.Sort(*s)
		*s = slices.Compact(*s)
	}
	slices.SortFunc(r.Refs, func(a, b Ref) int {
		return cmp.Or(cmp.Compare(a.Package, b.Package), cmp.Compare(a.Via, b.Via))
	})
	r.Refs = slices.Compact(r.Refs)
}

// ReadPending returns the names the file at path holds. An empty path, as
// PendingEnv unset gives, holds none.
func ReadPending(path string) (map[string]bool, error) {
	if path == "" {
		return nil, nil //nolint:nilnil // no names is no error
	}
	data, err := os.ReadFile(path) //nolint:gosec // the file the kanna command names
	if err != nil {
		return nil, fmt.Errorf("read what is pending: %w", err)
	}
	names := make(map[string]bool)
	for line := range strings.Lines(string(data)) {
		if name := strings.TrimSpace(line); name != "" {
			names[name] = true
		}
	}
	return names, nil
}
