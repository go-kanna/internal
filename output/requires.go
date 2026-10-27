package output

import (
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/go-kanna/internal/diag"
)

// MissingRequires warns when the file generated at path imports packages that
// the go.mod governing it does not require. The generator succeeds either way;
// it is the next go build that would stop, with "no required module provides
// package". imports are the paths the file imports from modules.
//
// A package counts as provided when its path is the module's own or lies under
// a module the go.mod requires. Inside a workspace there is nothing to warn
// about, because a module of the workspace provides packages no go.mod
// requires. The check is advice: anything that keeps it from reading go.mod
// skips it.
func MissingRequires(path string, imports []string) []diag.Diag {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil || inWorkspace(dir) {
		return nil
	}
	gomod := findUp(dir, "go.mod")
	if gomod == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Clean(gomod))
	if err != nil {
		return nil
	}
	f, err := modfile.ParseLax(gomod, data, nil)
	if err != nil || f.Module == nil {
		return nil
	}

	modules := []string{f.Module.Mod.Path}
	for _, r := range f.Require {
		modules = append(modules, r.Mod.Path)
	}
	var missing []string
	for _, imp := range imports {
		if !slices.ContainsFunc(modules, func(m string) bool { return imp == m || strings.HasPrefix(imp, m+"/") }) {
			missing = append(missing, imp)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []diag.Diag{diag.Warningf(token.Position{Filename: path},
		"imports %s, which go.mod does not require; run go mod tidy", strings.Join(missing, ", "))}
}

// inWorkspace reports whether the go command works in a workspace from dir:
// GOWORK names a file, or is unset and a go.work sits in dir or above it.
func inWorkspace(dir string) bool {
	switch gowork := os.Getenv("GOWORK"); gowork {
	case "off":
		return false
	case "":
		return findUp(dir, "go.work") != ""
	default:
		return true
	}
}

// findUp returns the path of the file called name in dir or the nearest
// directory above it that holds one, or "" when none does.
func findUp(dir, name string) string {
	for {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
