// Package qualifier tells which package a name such as model, in
// rule=model.CompletedAt or with=postgres.NewTodo, stands for. Go tells it for
// model.CompletedAt in code by the imports of the file it is written in, and so
// does this package, for a tag or a directive: there is no looking for a
// package of that name elsewhere.
//
// A package that only a tag names has to be imported as _, since Go rejects a
// named import nothing uses. A package imported as _ goes by its own name.
package qualifier

import (
	"fmt"
	"go/ast"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/go-kanna/internal/suggest"
)

// Scope is how one file calls the packages it imports: by the name it imports
// one under, or by the package's own name where it imports one as _ or without
// a name. A dot import gives no name.
type Scope struct {
	byName map[string][]*types.Package
}

// Of reads the imports of file, a file of pkg.
func Of(file *ast.File, pkg *types.Package) Scope {
	s := Scope{byName: make(map[string][]*types.Package)}
	if file == nil || pkg == nil {
		return s
	}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		i := slices.IndexFunc(pkg.Imports(), func(p *types.Package) bool { return p.Path() == path })
		if i < 0 {
			continue
		}
		imported := pkg.Imports()[i]
		name := imported.Name()
		if spec.Name != nil && spec.Name.Name != "_" {
			name = spec.Name.Name
		}
		if name != "." && !slices.Contains(s.byName[name], imported) {
			s.byName[name] = append(s.byName[name], imported)
		}
	}
	return s
}

// Error says why a name stands for no one package, and what to write instead.
type Error struct {
	Msg, Hint string
}

func (e *Error) Error() string { return e.Msg }

// Lookup returns the package name stands for in s.
func (s Scope) Lookup(name string) (*types.Package, error) {
	switch ps := s.byName[name]; len(ps) {
	case 1:
		return ps[0], nil
	case 0:
		e := &Error{
			Msg:  "the file imports no package named " + name,
			Hint: "import it, as _ if nothing else uses it",
		}
		if c, ok := suggest.Closest(name, slices.Sorted(maps.Keys(s.byName))); ok {
			e.Hint = fmt.Sprintf("did you mean %s?", c)
		}
		return nil, e
	default:
		paths := make([]string, len(ps))
		for i, p := range ps {
			paths[i] = p.Path()
		}
		return nil, &Error{
			Msg:  fmt.Sprintf("the file imports more than one package named %s: %s", name, strings.Join(paths, ", ")),
			Hint: "a package imported as _ goes by its own name, which these share",
		}
	}
}
